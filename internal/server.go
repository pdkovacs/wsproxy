package wsproxy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"
	"wsproxy/internal/config"
	"wsproxy/internal/logging"

	"github.com/gin-gonic/gin"
	"github.com/rs/xid"
	"github.com/rs/zerolog"
)

type EndpointPath string

const (
	ConnectPath     EndpointPath = "/connect"
	DisonnectedPath EndpointPath = "/disconnected"
	MessagePath     EndpointPath = "/message"
)

type Server struct {
	Addr               string
	createConnectionId func() ConnectionID
	clusterSupport     *ClusterSupport
	server             http.Server
	configuration      config.Config
	ctx                context.Context
}

func NewServer(
	ctx context.Context,
	configuration config.Config,
	createConnectionId func() ConnectionID,
) *Server {
	return &Server{
		configuration:      configuration,
		createConnectionId: createConnectionId,
		clusterSupport:     NewClusterSupport(configuration),
		ctx:                ctx,
	}
}

// start starts the service
func (s *Server) start(r http.Handler, ready func(port int, stop func())) error {
	logger := zerolog.Ctx(s.ctx).With().Str("method", "start").Logger()
	logger.Info().Msg("Starting server on ephemeral....")

	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", s.configuration.ServerHost, s.configuration.ServerPort))
	if err != nil {
		panic(fmt.Sprintf("Error while starting to listen at an ephemeral port: %v", err))
	}
	s.Addr = listener.Addr().String()
	logger.Info().Msgf("wsproxy instance is listening at %s", s.Addr)

	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		panic(fmt.Sprintf("Error while parsing the server address: %v", err))
	}

	logger.Info().Msgf("Listening on port: %v", port)

	if ready != nil {
		portAsInt, err := strconv.Atoi(port)
		if err != nil {
			panic(err)
		}
		ready(portAsInt, s.Stop)
	}

	server := &http.Server{
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	return server.Serve(listener)
}

// SetupAndStart sets up and starts server.
func (s *Server) SetupAndStart(ready func(port int, stop func())) error {
	r := createWsproxyRequestHandler(s.configuration, s.createConnectionId, s.clusterSupport)
	return s.start(r, ready)
}

// For now, we assume that the backend authentication is managed ex-machina by the environment (AWS role or K8S NetworkPolicy
// or by a service-mesh provider)
// In the unlikely case of ex-machina control isn't available, OAuth2 client credentials flow could be easily supported.
// (Use https://pkg.go.dev/github.com/golang-jwt/jwt/v4#example-package-GetTokenViaHTTP to verify the token.)
func authenticateBackend(c *gin.Context) error {
	return nil
}

// Stop kills the listener
func (s *Server) Stop() {
	logger := zerolog.Ctx(s.ctx).With().Str("method", "stop").Logger()
	logger.Info().Msgf("Shutting down server...")
	error := s.server.Shutdown(s.ctx)
	if error != nil {
		logger.Error().Msgf("Error while shutting down server: %v", error)
	} else {
		logger.Info().Msg("Server shutdown successfully")
	}
}

func createWsproxyRequestHandler(options config.Config, createConnectionId func() ConnectionID, clusterSupport *ClusterSupport) *gin.Engine {
	rootEngine := gin.Default()

	rootEngine.Use(RequestLogger("websocketGatewayServer"))

	wsConns := newWsConnections()

	appUrls := appURLs{
		baseUrl: options.AppBaseUrl,
	}

	rootEngine.GET(
		string(ConnectPath),
		connectHandler(
			&appUrls,
			wsConns,
			options.LoadBalancerAddress,
			createConnectionId,
			clusterSupport,
		),
	)

	rootEngine.POST(
		fmt.Sprintf("/message/:%s", connIdPathParamName),
		pushHandler(
			authenticateBackend,
			wsConns,
			clusterSupport,
		),
	)

	return rootEngine
}

type appURLs struct {
	baseUrl string
}

func (u *appURLs) connecting() string {
	return fmt.Sprintf("%s/ws%s", u.baseUrl, ConnectPath)
}

func (u *appURLs) disconnected() string {
	return fmt.Sprintf("%s/ws%s", u.baseUrl, DisonnectedPath)
}

func (u *appURLs) message() string {
	return fmt.Sprintf("%s/ws%s", u.baseUrl, MessagePath)
}

func RequestLogger(unitName string) func(g *gin.Context) {
	return func(g *gin.Context) {
		start := time.Now()

		l := logging.Get().With().Str("req_xid", xid.New().String()).Logger()

		r := g.Request
		g.Request = r.WithContext(l.WithContext(r.Context()))

		lrw := newLoggingResponseWriter(g.Writer)

		defer func() {
			panicVal := recover()
			if panicVal != nil {
				lrw.statusCode = http.StatusInternalServerError // ensure that the status code is updated
				panic(panicVal)                                 // continue panicking
			}
			l.
				Info().
				Str("unit", unitName).
				Str("method", g.Request.Method).
				Str("url", g.Request.URL.RequestURI()).
				Str("user_agent", g.Request.UserAgent()).
				Int("status_code", lrw.statusCode).
				Dur("elapsed_ms", time.Since(start)).
				Msg("incoming request")
		}()

		g.Next()
	}
}

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func newLoggingResponseWriter(w http.ResponseWriter) *loggingResponseWriter {
	return &loggingResponseWriter{w, http.StatusOK}
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}
