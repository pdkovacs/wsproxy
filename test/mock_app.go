package test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	wsproxy "wsproxy/internal"
	"wsproxy/internal/logging"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/mock"
)

const badCredential = "bad-credential"

const (
	mockMethodConnect         = "connect"
	mockMethodDisconnected    = "disconnected"
	mockMethodMessageReceived = "messageReceived"
)

type messageJSON map[string]string

type MyMock struct {
	disconnectNotification chan struct{}
	mock.Mock
}

func (m *MyMock) connect(headerKey string, connectionId string) {
	m.Called(headerKey, connectionId)
}

func (m *MyMock) disconnected(headerKey string, connectionId string) {
	m.Called(headerKey, connectionId)
	m.disconnectNotification <- struct{}{}
}

func (m *MyMock) messageReceived(headerKey string, connectionId string, msg messageJSON) {
	m.Called(headerKey, connectionId, msg)
}

type mockApplication struct {
	// getWsproxyUrl makes available the URL of the WSGS server
	getWsproxyUrl func() string
	listener   net.Listener
	stop       func()
	logger     zerolog.Logger
	mockMock   *MyMock
}

func newMockApp(getWsproxyUrl func() string) *mockApplication {
	return &mockApplication{
		getWsproxyUrl: getWsproxyUrl,
		logger:     logging.Get().With().Str("unit", "mockApplication").Logger(),
		mockMock:   &MyMock{disconnectNotification: make(chan struct{})},
	}
}

func (m *mockApplication) start() error {
	address := fmt.Sprintf(":%d", 0)
	listener, listenErr := net.Listen("tcp", address)
	if listenErr != nil {
		return fmt.Errorf("failed to listen on %s: %w", address, listenErr)
	}
	m.listener = listener

	handler, creHandlerErr := m.createMockAppRequestHandler()
	if creHandlerErr != nil {
		return fmt.Errorf("failed to create mockApp request handler: %w", creHandlerErr)
	}

	go func() {
		serveErr := http.Serve(listener, handler)
		if serveErr != nil {
			m.logger.Error().Err(serveErr)
		}
	}()

	m.stop = func() {
		listener.Close()
	}

	return nil
}

func (m *mockApplication) resetCalls() {
	m.mockMock.ExpectedCalls = m.mockMock.ExpectedCalls[:0]
	m.mockMock.Calls = m.mockMock.Calls[:0]
	m.mockMock.disconnectNotification = make(chan struct{})
}

func (m *mockApplication) createMockAppRequestHandler() (http.Handler, error) {
	rootEngine := gin.Default()
	rootEngine.Use(wsproxy.RequestLogger("mockApplication"))

	ws := rootEngine.Group("/ws")

	ws.GET(string(wsproxy.ConnectPath), func(g *gin.Context) {
		logger := zerolog.Ctx(g.Request.Context()).With().Str("method", "connect-handler").Logger()
		req := g.Request
		res := g
		cred, hasCredHeader := req.Header["Authorization"]
		logger.Debug().Msgf("Request has authorization header: %v and it is: %s", hasCredHeader, cred)
		if !hasCredHeader {
			_ = res.AbortWithError(500, errors.New("authorization header not found"))
			return
		}
		if cred[0] == badCredential {
			_ = res.AbortWithError(401, errors.New("bad credentials in Authorization header"))
			return
		}

		connHeaderKey := wsproxy.ConnectionIDHeaderKey
		if connId := req.Header.Get(connHeaderKey); connId != "" {
			m.mockMock.connect(connHeaderKey, connId)
		}

		res.Status(200)
	})

	ws.POST(string(wsproxy.DisonnectedPath), func(g *gin.Context) {
		req := g.Request

		connHeaderKey := wsproxy.ConnectionIDHeaderKey
		if connId := req.Header.Get(connHeaderKey); connId != "" {
			m.mockMock.disconnected(connHeaderKey, connId)
		}
	})

	ws.POST(string(wsproxy.MessagePath), func(g *gin.Context) {
		logger := zerolog.Ctx(g.Request.Context()).With().Str("method", "message handler").Logger()
		req := g.Request
		connHeaderKey := wsproxy.ConnectionIDHeaderKey
		if connId := req.Header.Get(connHeaderKey); connId != "" {
			bodyAsBytes, readBodyErr := io.ReadAll(req.Body)
			if readBodyErr != nil {
				logger.Error().Err(readBodyErr).Send()
				return
			}
			m.mockMock.messageReceived(connHeaderKey, connId, parseMessageJSON(bodyAsBytes))
		}
	})

	return rootEngine, nil
}

func parseMessageJSON(value []byte) messageJSON {
	message := map[string]string{}
	unmarshalErr := json.Unmarshal(value, &message)
	if unmarshalErr != nil {
		panic(unmarshalErr)
	}
	return message
}
