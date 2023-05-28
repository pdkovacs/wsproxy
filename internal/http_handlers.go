package wsgw

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
	"websocket-gateway/internal/logging"

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
}

// Relays the connection request to the backend's `POST /ws/connect` endpoint and
func notifyAppOfWsConnecting(appUrls applicationURLs, connId ConnectionID) func(c *gin.Context) bool {

	return func(c *gin.Context) bool {
		logger := zerolog.Ctx(c).With().Str("method", fmt.Sprintf("notifyAppOfWsConnecting: %s", appUrls.connecting())).Logger()

		request, err := http.NewRequest(http.MethodGet, appUrls.connecting(), nil)
		if err != nil {
			logger.Error().Msgf("failed to create request object: %v", err)
			c.AbortWithStatus(http.StatusInternalServerError)
			return false
		}
		request.Header = c.Request.Header

		request.Header.Add(ConnectionIDHeaderKey, string(connId))

		// TODO: doesn't recreate the client for each request
		client := http.Client{
			Timeout: time.Second * 15,
		}
		response, requestErr := client.Do(request)
		if requestErr != nil {
			logger.Error().Msgf("failed to send request: %v", requestErr)
			c.AbortWithStatus(http.StatusInternalServerError)
			return false
		}
		if response.StatusCode == http.StatusUnauthorized {
			logger.Info().Msg("Authentication failed")
			c.AbortWithStatus(http.StatusUnauthorized)
			return false
		}
		if response.StatusCode != 200 {
			logger.Info().Msgf("Received status code %d", response.StatusCode)
			c.AbortWithStatus(http.StatusInternalServerError)
			return false
		}
		return true
	}
}

func notifyAppOfWsDisconnected(appUrls applicationURLs, connId ConnectionID, logger zerolog.Logger) {
	logger = logging.CreateMethodLogger(logger, "notifyAppOfWsDisconnected").With().Str("appUrl", appUrls.disconnected()).Str("connectionId", string(connId)).Logger()

	logger.Debug().Msg("BEGIN")

	request, err := http.NewRequest(http.MethodPost, appUrls.disconnected(), nil)
	if err != nil {
		logger.Error().Msgf("failed to create request object: %v", err)
		return
	}
	request.Header.Add(ConnectionIDHeaderKey, string(connId))

	// TODO: doesn't recreate the client for each request
	client := http.Client{
		Timeout: time.Second * 15,
	}
	response, requestErr := client.Do(request)
	if requestErr != nil {
		logger.Error().Msgf("failed to send request: %v", requestErr)
		return
	}
	if response.StatusCode != 200 {
		logger.Info().Msgf("Received status code %d", response.StatusCode)
		return
	}
}

// connectHandler calls `authenticateClient` if it is not `nil` to authenticate the client,
// then notifies the application of the new WS connection
func connectHandler(
	appUrls applicationURLs,
	ws *wsConnections,
	loadBalancerAddress string,
	createConnectionId func() ConnectionID,
	onMessageReceived onMgsReceivedFunc,
) gin.HandlerFunc {

	return func(g *gin.Context) {
		logger := zerolog.Ctx(g).With().Str("method", "connectHandler").Logger()

		connId := createConnectionId()

		appAccepted := notifyAppOfWsConnecting(appUrls, connId)(g)

		logger.Debug().Msgf("app has accepted: %v", appAccepted)

		if !appAccepted {
			return
		}

		wsConn, subsErr := websocket.Accept(g.Writer, g.Request, &websocket.AcceptOptions{
			OriginPatterns: []string{loadBalancerAddress},
		})
		if subsErr != nil {
			logger.Error().Msgf("Failed to accept WS connection request: %v", subsErr)
			g.Error(subsErr)
			g.AbortWithStatus(500)
			return
		}
		defer wsConn.Close(websocket.StatusNormalClosure, "")

		logger.Debug().Msg("websocket message processing about to start...")
		wsClosedError := ws.processMessages(g.Request.Context(), connId, &wsIOAdapter{wsConn}, onMessageReceived) // we block here until Error or Done
		logger.Debug().Msgf("websocket message processing finished with %v", wsClosedError)

		notifyAppOfWsDisconnected(appUrls, connId, logger)

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
		logger := zerolog.Ctx(g).With().Str("method", "pushHandler").Logger()

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

		errPush := ws.push(bodyAsString, ConnectionID(connectionIdStr))
		if errPush == errConnectionNotFound {
			logger.Error().Msgf("Connection doesn't exist: %s:", connectionIdStr)
			g.AbortWithStatus(http.StatusNotFound)
			return
		}

		logger.Error().Msgf("Failed to push to connection %s: %v", connectionIdStr, errPush)
		g.AbortWithStatus(http.StatusInternalServerError)
	}
}
