package test

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	wsgw "websocket-gateway/internal"
	"websocket-gateway/internal/logging"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/mock"
)

const badCredential = "bad-credential"

const (
	mockMethodConnecting   = "connecting"
	mockMethodDisconnected = "disconnected"
)

type MyMock struct {
	disconnectListener chan struct{}
	mock.Mock
}

func (m *MyMock) connecting(headerKey string, connectionId string) {
	m.Called(headerKey, connectionId)
}

func (m *MyMock) disconnected(headerKey string, connectionId string) {
	m.Called(headerKey, connectionId)
	m.disconnectListener <- struct{}{}
}

type mockApplication struct {
	// getWsgwUrl makes available the URL of the WSGS server
	getWsgwUrl func() string
	listener   net.Listener
	stop       func()
	logger     zerolog.Logger
	mockMock   *MyMock
}

func newMockApp(getWsgwUrl func() string) *mockApplication {
	return &mockApplication{
		getWsgwUrl: getWsgwUrl,
		logger:     logging.Get().With().Str("unit", "mockApplication").Logger(),
		mockMock:   &MyMock{disconnectListener: make(chan struct{})},
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
		http.Serve(listener, handler)
	}()

	m.stop = func() {
		listener.Close()
	}

	return nil
}

func (m *mockApplication) resetCalls() {
	m.mockMock.ExpectedCalls = m.mockMock.ExpectedCalls[:0]
	m.mockMock.Calls = m.mockMock.Calls[:0]
	m.mockMock.disconnectListener = make(chan struct{})
}

func (m *mockApplication) createMockAppRequestHandler() (http.Handler, error) {
	rootEngine := gin.Default()
	rootEngine.Use(wsgw.RequestLogger("mockApplication"))

	ws := rootEngine.Group("/ws")

	ws.GET(string(wsgw.ConnectPath), func(g *gin.Context) {
		logger := logging.CreateMethodLogger(m.logger, "authentication handler")
		req := g.Request
		res := g
		cred, hasCredHeader := req.Header["Authorization"]
		logger.Debug().Msgf("Request has authorization header: %v and it is: %s", hasCredHeader, cred)
		if !hasCredHeader {
			res.AbortWithError(500, errors.New("authorization header not found"))
			return
		}
		if cred[0] == badCredential {
			res.AbortWithError(401, errors.New("bad credentials in Authorization header"))
			return
		}

		connHeaderKey := wsgw.ConnectionIDHeaderKey
		if connId := req.Header.Get(connHeaderKey); connId != "" {
			m.mockMock.connecting(connHeaderKey, connId)
		}

		res.Status(200)
	})

	ws.POST(string(wsgw.DisonnectedPath), func(g *gin.Context) {
		req := g.Request

		connHeaderKey := wsgw.ConnectionIDHeaderKey
		if connId := req.Header.Get(connHeaderKey); connId != "" {
			m.mockMock.disconnected(connHeaderKey, connId)
		}
	})

	ws.POST("/message-received")

	return rootEngine, nil
}
