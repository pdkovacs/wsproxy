package wsproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"nhooyr.io/websocket"
)

// TODO: make this configurable?
const ConnectionIDHeaderKey = "X-WSGW-CONNECTION-ID"

type wsIOAdapter struct {
	wsConn *websocket.Conn
}

func (wsIo *wsIOAdapter) CloseRead(ctx context.Context) context.Context {
	return wsIo.wsConn.CloseRead(ctx)
}

func (wsIo *wsIOAdapter) Close() error {
	return wsIo.wsConn.Close(websocket.StatusPolicyViolation, "connection too slow to keep up with messages")
}

func (wsIo *wsIOAdapter) Write(ctx context.Context, msg string) error {
	return wsIo.wsConn.Write(ctx, websocket.MessageText, []byte(msg))
}

func (wsIo *wsIOAdapter) Read(ctx context.Context) (string, error) {
	msgType, msg, err := wsIo.wsConn.Read(ctx)
	if err != nil {
		return "", err
	}
	if msgType != websocket.MessageText {
		return "", errors.New("unexpected message type")
	}
	return string(msg), nil
}

type applicationURLs interface {
	connecting() string
	disconnected() string
	message() string
}

type appConnection struct {
	id         ConnectionID
	httpClient http.Client
}

// Relays the connection request to the backend's `POST /ws/connect` endpoint and
func handleClientConnecting(createConnectionId func() ConnectionID, appUrls applicationURLs) func(c *gin.Context) *appConnection {
	return func(g *gin.Context) *appConnection {
		logger := zerolog.Ctx(g.Request.Context()).With().Str("method", fmt.Sprintf("handleClientConnecting: %s", appUrls.connecting())).Logger()

		request, err := http.NewRequest(http.MethodGet, appUrls.connecting(), nil)
		if err != nil {
			logger.Error().Msgf("failed to create request object: %v", err)
			g.AbortWithStatus(http.StatusInternalServerError)
			return nil
		}
		request.Header = g.Request.Header

		connId := createConnectionId()

		request.Header.Add(ConnectionIDHeaderKey, string(connId))

		client := http.Client{
			Timeout: time.Second * 15,
		}
		response, requestErr := client.Do(request)
		if requestErr != nil {
			logger.Error().Msgf("failed to send request: %v", requestErr)
			g.AbortWithStatus(http.StatusInternalServerError)
			return nil
		}
		defer cleanupResponse(response)

		if response.StatusCode == http.StatusUnauthorized {
			logger.Info().Msg("Authentication failed")
			g.AbortWithStatus(http.StatusUnauthorized)
			return nil
		}

		if response.StatusCode != 200 {
			logger.Info().Msgf("Received status code %d", response.StatusCode)
			g.AbortWithStatus(http.StatusInternalServerError)
			return nil
		}

		logger.Debug().Msgf("app has accepted: %v", connId)

		return &appConnection{connId, client}
	}
}

func handleClientDisconnected(appUrls applicationURLs, appConn *appConnection, logger zerolog.Logger) {
	logger = logger.With().Str("method", "handleClientDisconnected").Str("appUrl", appUrls.disconnected()).Str("connectionId", string(appConn.id)).Logger()

	logger.Debug().Msg("BEGIN")

	request, err := http.NewRequest(http.MethodPost, appUrls.disconnected(), nil)
	if err != nil {
		logger.Error().Msgf("failed to create request object: %v", err)
		return
	}
	request.Header.Add(ConnectionIDHeaderKey, string(appConn.id))

	response, requestErr := appConn.httpClient.Do(request)
	if requestErr != nil {
		logger.Error().Msgf("failed to send request: %v", requestErr)
		return
	}
	defer cleanupResponse(response)

	if response.StatusCode != 200 {
		logger.Info().Msgf("Received status code %d", response.StatusCode)
		return
	}
}

// Calls the `POST /ws/message-received` endpoint on the backend with "msg" and "connectionId"
func handleClientMessage(appConn *appConnection, appUrls applicationURLs) func(c context.Context, msg string) error {
	return func(c context.Context, msg string) error {
		logger := zerolog.Ctx(c).With().Str("connectionId", string(appConn.id)).Str("func", "handleClientMessage").Logger()
		logger.Debug().Str("msg", msg).Send()

		request, err := http.NewRequest(
			http.MethodPost,
			appUrls.message(),
			bytes.NewReader([]byte(msg)),
		)
		if err != nil {
			logger.Error().Msgf("failed to create request object: %v", err)
			return err
		}
		request.Header.Add(ConnectionIDHeaderKey, string(appConn.id))

		response, requestErr := appConn.httpClient.Do(request)
		if requestErr != nil {
			logger.Error().Msgf("failed to send request: %v", requestErr)
			return requestErr
		}
		defer cleanupResponse(response)

		if response.StatusCode != 200 {
			logger.Info().Msgf("Received status code %d", response.StatusCode)
			return fmt.Errorf("probelm while sending message to application")
		}

		return nil
	}
}

// connectHandler calls `authenticateClient` if it is not `nil` to authenticate the client,
// then notifies the application of the new WS connection
func connectHandler(
	appUrls applicationURLs,
	ws *wsConnections,
	loadBalancerAddress string,
	createConnectionId func() ConnectionID,
) gin.HandlerFunc {
	return func(g *gin.Context) {
		appConn := handleClientConnecting(createConnectionId, appUrls)(g)

		if appConn == nil {
			return
		}

		logger := zerolog.Ctx(g.Request.Context()).With().Str("method", "authentication handler").Logger()

		// logger = logger.().Str("method", "connectHandler").Str("connectionId", string(appConn.id)).Logger()

		wsConn, subsErr := websocket.Accept(g.Writer, g.Request, &websocket.AcceptOptions{
			OriginPatterns: []string{loadBalancerAddress},
		})
		if subsErr != nil {
			logger.Error().Msgf("Failed to accept WS connection request: %v", subsErr)
			_ = g.Error(subsErr)
			g.AbortWithStatus(500)
			return
		}
		defer wsConn.Close(websocket.StatusNormalClosure, "")

		logger.Debug().Msg("websocket message processing about to start...")

		wsClosedError := ws.processMessages(g.Request.Context(), logger, &wsIOAdapter{wsConn}, handleClientMessage(appConn, appUrls)) // we block here until Error or Done
		logger.Debug().Msgf("websocket message processing finished with %v", wsClosedError)

		handleClientDisconnected(appUrls, appConn, logger)

		if errors.Is(wsClosedError, context.Canceled) {
			return // Done
		}

		if websocket.CloseStatus(wsClosedError) == websocket.StatusNormalClosure ||
			websocket.CloseStatus(wsClosedError) == websocket.StatusGoingAway {
			return
		}

		if wsClosedError != nil {
			logger.Error().Msgf("%v", wsClosedError)
			return
		}
	}
}

func pushHandler(authenticateBackend func(c *gin.Context) error, connIdPathParamName string, ws *wsConnections) gin.HandlerFunc {
	return func(g *gin.Context) {
		logger := zerolog.Ctx(g.Request.Context()).With().Str("method", "pushHandler").Logger()

		connectionIdStr := g.Param(connIdPathParamName)
		if connectionIdStr == "" {
			logger.Info().Msgf("Missing path param: %s", connIdPathParamName)
			g.AbortWithStatus(http.StatusBadRequest)
			return
		}

		requestBody, errReadRequest := io.ReadAll(g.Request.Body)
		if errReadRequest != nil {
			logger.Error().Msgf("failed to read request body %T: %v", g.Request.Body, errReadRequest)
			g.JSON(500, nil)
			return
		}
		var body interface{}
		errBodyUnmarshal := json.Unmarshal(requestBody, &body)
		if errBodyUnmarshal != nil {
			logger.Error().Msgf("failed to unmarshal request body %T: %v", requestBody, errBodyUnmarshal)
			g.JSON(400, nil)
			return
		}

		bodyAsString, conversionOk := body.(string)
		if !conversionOk {
			logger.Error().Msgf("failed to convert request body tp string: %T", requestBody)
			g.JSON(400, nil)
			return
		}

		errPush := ws.push(g.Request.Context(), bodyAsString, ConnectionID(connectionIdStr))
		if errPush == errConnectionNotFound {
			logger.Error().Msgf("Connection doesn't exist: %s:", connectionIdStr)
			g.AbortWithStatus(http.StatusNotFound)
			return
		}

		logger.Error().Msgf("Failed to push to connection %s: %v", connectionIdStr, errPush)
		g.AbortWithStatus(http.StatusInternalServerError)
	}
}

func cleanupResponse(response *http.Response) {
	_, _ = io.Copy(io.Discard, response.Body)
	response.Body.Close()
}
