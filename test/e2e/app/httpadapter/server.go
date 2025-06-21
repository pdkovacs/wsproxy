package httpadapter

import (
	"database/sql"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
	wsproxy "wsproxy/internal"
	"wsproxy/internal/logging"
	"wsproxy/test/e2e/app/config"
	"wsproxy/test/e2e/app/security/authn"
	"wsproxy/test/e2e/app/services"
	"wsproxy/test/e2e/app/web"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/memstore"
	"github.com/gin-contrib/sessions/postgres"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

const BadCredential = "bad-credential"

const (
	MockMethodConnect         = "connect"
	MockMethodDisconnected    = "disconnected"
	MockMethodMessageReceived = "messageReceived"
)

type MessageJSON map[string]string

type server struct {
	listener      net.Listener
	configuration config.Options
	getWsproxyUrl func() string
	logger        zerolog.Logger
}

func NewServer(configuration config.Options, getWsproxyUrl func() string) server {
	return server{
		configuration: configuration,
		getWsproxyUrl: getWsproxyUrl,
		logger:        logging.Get().With().Str(logging.UnitLogger, "http-server").Logger(),
	}
}

// start starts the service
func (s *server) start(portRequested int, r http.Handler, ready func(port int, stop func())) {
	logger := s.logger.With().Str(logging.MethodLogger, "StartServer").Logger()
	logger.Info().Msg("Starting server on ephemeral....")
	var err error

	s.listener, err = net.Listen("tcp", fmt.Sprintf(":%d", portRequested))
	if err != nil {
		panic(fmt.Sprintf("Error while starting to listen at an ephemeral port: %v", err))
	}

	_, port, err := net.SplitHostPort(s.listener.Addr().String())
	if err != nil {
		panic(fmt.Sprintf("Error while parsing the server address: %v", err))
	}

	logger.Info().Str("port", port).Msg("started to listen")

	if ready != nil {
		portAsInt, err := strconv.Atoi(port)
		if err != nil {
			panic(err)
		}
		ready(portAsInt, s.Stop)
	}

	http.Serve(s.listener, r)
}

// SetupAndStart sets up and starts server.
func (s *server) Start(options config.Options, ready func(port int, stop func())) {
	r := s.initEndpoints(options)
	s.start(options.ServerPort, r, ready)
}

func (s *server) createSessionStore(options config.Options) (sessions.Store, error) {
	var store sessions.Store
	logger := s.logger.With().Str(logging.MethodLogger, "create-session properties").Logger()

	if options.SessionDbName == "" {
		logger.Info().Msg("Using in-memory session store")
		store = memstore.NewStore([]byte("secret"))
	} else if len(options.DynamodbURL) > 0 {
		panic("DynamoDB session store is not supported yet")
	} else if len(options.DBHost) > 0 {
		logger.Info().Str("database", options.SessionDbName).Msg("connecting to session store")
		connProps := config.CreateDbProperties(s.configuration, logger)
		connStr := fmt.Sprintf(
			"postgres://%s:%s@%s:%d/%s?sslmode=disable",
			connProps.User,
			connProps.Password,
			connProps.Host,
			connProps.Port,
			options.SessionDbName,
		)
		sessionDb, openSessionDbErr := sql.Open("pgx", connStr)
		if openSessionDbErr != nil {
			return store, openSessionDbErr
		}
		sessionDb.Ping()
		var createDbSessionStoreErr error
		store, createDbSessionStoreErr = postgres.NewStore(sessionDb, []byte("secret"))
		if createDbSessionStoreErr != nil {
			return store, createDbSessionStoreErr
		}
	}

	return store, nil
}

func (s *server) initEndpoints(options config.Options) *gin.Engine {
	logger := s.logger.With().Str(logging.MethodLogger, "server:initEndpoints").Logger()
	authorizationService := services.NewAuthorizationService(options)
	userService := services.NewUserService(&authorizationService)

	rootEngine := gin.Default()
	rootEngine.Use(wsproxy.RequestLogger("e2etest-application"))

	if options.AuthenticationType != authn.SchemeOIDCProxy {
		gob.Register(SessionData{})
		store, createStoreErr := s.createSessionStore(options)
		if createStoreErr != nil {
			panic(createStoreErr)
		}
		store.Options(sessions.Options{MaxAge: options.SessionMaxAge})
		rootEngine.Use(sessions.Sessions("mysession", store))
	}

	rootEngine.NoRoute(authentication(options, &userService, s.logger.With().Logger()), gin.WrapH(web.AssetHandler("/", "dist", logger)))

	logger.Debug().Str("authenticationType", string(options.AuthenticationType)).Msg("Creating login end-point...")
	rootEngine.GET("/login", authentication(options, &userService, s.logger.With().Logger()))

	rootEngine.GET("/app-info", func(c *gin.Context) {
		c.JSON(200, config.GetBuildInfo())
	})

	logger.Debug().Msg("Creating authorized group....")

	// mustGetUserInfo := func(c *gin.Context) authr.UserInfo {
	// 	userInfo, getUserInfoErr := getUserInfo(options.AuthenticationType)(c)
	// 	if getUserInfoErr != nil {
	// 		panic(fmt.Sprintf("failed to get user-info %s", c.Request.URL))
	// 	}
	// 	return userInfo
	// }

	authorizedGroup := rootEngine.Group("/")
	{
		logger.Debug().Str("authn-type", string(options.AuthenticationType)).Msg("Setting up authorized group")
		authorizedGroup.Use(authenticationCheck(options, &userService))

		rootEngine.GET("/config", func(c *gin.Context) {
			c.JSON(200, clientConfig{IdPLogoutURL: options.OIDCLogoutURL})
		})
		logger.Debug().Msg("Setting up logout handler")
		authorizedGroup.POST("/logout", logout(options))

		authorizedGroup.GET("/user", userInfoHandler(options.AuthenticationType, userService))

		authorizedGroup.GET("/users", userListHandler(userService))

		if options.EnableBackdoors {
			authorizedGroup.PUT("/backdoor/authentication", HandlePutIntoBackdoorRequest())
			authorizedGroup.GET("/backdoor/authentication", HandleGetIntoBackdoorRequest())
		}

		ws := authorizedGroup.Group("/ws")
		ws.GET(string(wsproxy.ConnectPath), func(g *gin.Context) {
			logger := zerolog.Ctx(g.Request.Context()).With().Str("method", "WS connect handler").Logger()
			req := g.Request
			res := g
			cred, hasCredHeader := req.Header["Authorization"]
			logger.Debug().Msgf("Request has authorization header: %v and it is: %s", hasCredHeader, cred)
			if !hasCredHeader {
				_ = res.AbortWithError(500, errors.New("authorization header not found"))
				return
			}
			if cred[0] == BadCredential {
				_ = res.AbortWithError(401, errors.New("bad credentials in Authorization header"))
				return
			}

			connHeaderKey := wsproxy.ConnectionIDHeaderKey
			if connId := req.Header.Get(connHeaderKey); connId != "" {
				logger.Info().Str(wsproxy.ConnectionIDKey, connId).Str("connid", connId).Msg("Incoming connection request...")
				res.Status(200)
				return
			}

			res.Status(200)
		})

		ws.POST(string(wsproxy.DisonnectedPath), func(g *gin.Context) {
			logger := zerolog.Ctx(g.Request.Context()).With().Str("method", "WS disconnection handler").Logger()
			req := g.Request
			res := g

			connHeaderKey := wsproxy.ConnectionIDHeaderKey
			if connId := req.Header.Get(connHeaderKey); connId != "" {
				logger.Error().Str(wsproxy.ConnectionIDKey, connId).Str("connid", connId).Msg("Disconnect requested")
				res.Status(200)
				return
			}
		})

		ws.POST(string(wsproxy.MessagePath), func(g *gin.Context) {
			logger := zerolog.Ctx(g.Request.Context()).With().Str("method", "WS message handler").Logger()
			req := g.Request
			connHeaderKey := wsproxy.ConnectionIDHeaderKey
			if connId := req.Header.Get(connHeaderKey); connId != "" {
				bodyAsBytes, readBodyErr := io.ReadAll(req.Body)
				req.Body.Close()
				if readBodyErr != nil {
					logger.Error().Err(readBodyErr).Send()
					return
				}

				message := parseMessageJSON(bodyAsBytes)
				logger.Debug().Str(wsproxy.ConnectionIDKey, connId).Any("message", message).Msg("message received")
			}
		})

	}

	return rootEngine
}

// Stop kills the listener
func (s *server) Stop() {
	logger := s.logger.With().Str(logging.MethodLogger, "ListenerKiller").Logger()
	error := s.listener.Close()
	if error != nil {
		logger.Error().Err(error).Interface("listener", s.listener).Msg("Error while closing listener")
	} else {
		logger.Info().Interface("listener", s.listener).Msg("Listener closed successfully")
	}
}

func (s *server) SendToClient(connId wsproxy.ConnectionID, message MessageJSON) error {
	url := fmt.Sprintf("%s%s/%s", s.getWsproxyUrl(), wsproxy.MessagePath, connId)
	req, createReqErr := http.NewRequest(http.MethodPost, url, strings.NewReader(message["message"]))
	if createReqErr != nil {
		return createReqErr
	}
	client := http.Client{
		Timeout: time.Second * 15,
	}
	response, sendReqErr := client.Do(req)
	if sendReqErr != nil {
		return sendReqErr
	}

	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("sending message to client finished with unexpected HTTP status: %v", response.StatusCode)
	}

	return nil
}

func parseMessageJSON(value []byte) MessageJSON {
	message := map[string]string{}
	unmarshalErr := json.Unmarshal(value, &message)
	if unmarshalErr != nil {
		panic(unmarshalErr)
	}
	return message
}
