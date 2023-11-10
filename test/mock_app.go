package test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
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

func newClientPeer() *MyMock {
	return &MyMock{disconnectNotification: make(chan struct{})}
}

func (m *MyMock) connect() {
	m.Called()
}

func (m *MyMock) disconnected() {
	m.Called()
	m.disconnectNotification <- struct{}{}
}

func (m *MyMock) messageReceived(msg messageJSON) {
	m.Called(msg)
}

type mockApplication struct {
	// getWsproxyUrl makes available the URL of the WSGS server
	getWsproxyUrl              func() string
	listener                   net.Listener
	stop                       func()
	logger                     zerolog.Logger
	connMocks                  map[string]*MyMock
	connMocksMux               sync.Mutex
	recordConnectionFromClient bool
}

func newMockApp(getWsproxyUrl func() string) *mockApplication {
	return &mockApplication{
		getWsproxyUrl: getWsproxyUrl,
		logger:        logging.Get().With().Str("unit", "mockApplication").Logger(),
		connMocks:     make(map[string]*MyMock),
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

func (m *mockApplication) createMockAppRequestHandler() (http.Handler, error) {
	rootEngine := gin.Default()
	rootEngine.Use(wsproxy.RequestLogger("mockApplication"))

	ws := rootEngine.Group("/ws")

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
		if cred[0] == badCredential {
			_ = res.AbortWithError(401, errors.New("bad credentials in Authorization header"))
			return
		}

		connHeaderKey := wsproxy.ConnectionIDHeaderKey
		if connId := req.Header.Get(connHeaderKey); connId != "" {
			m.connMocksMux.Lock()
			defer m.connMocksMux.Unlock()
			_, ok := m.connMocks[connId]
			if !ok {
				if m.recordConnectionFromClient {
					logger.Error().Str("connectionId", connId).Msg("connection not mocked")
				}
				return
			}
			m.connMocks[connId].connect()
		}

		res.Status(200)
	})

	ws.POST(string(wsproxy.DisonnectedPath), func(g *gin.Context) {
		logger := zerolog.Ctx(g.Request.Context()).With().Str("method", "WS disconnection handler").Logger()
		req := g.Request

		connHeaderKey := wsproxy.ConnectionIDHeaderKey
		if connId := req.Header.Get(connHeaderKey); connId != "" {
			m.connMocksMux.Lock()
			defer m.connMocksMux.Unlock()
			if _, ok := m.connMocks[connId]; !ok {
				logger.Error().Str("connectionId", connId).Msg("connection not mocked")
				return
			}
			m.connMocks[connId].disconnected()
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

			m.connMocksMux.Lock()
			defer m.connMocksMux.Unlock()
			if _, ok := m.connMocks[connId]; !ok {
				logger.Error().Str("connectionId", connId).Msg("connection not mocked")
				return
			}
			m.connMocks[connId].messageReceived(parseMessageJSON(bodyAsBytes))
		}
	})

	return rootEngine, nil
}

func (m *mockApplication) on(methodName string, connId wsproxy.ConnectionID, arguments ...any) {
	m.connMocksMux.Lock()
	defer m.connMocksMux.Unlock()
	if _, ok := m.connMocks[string(connId)]; !ok {
		m.connMocks[string(connId)] = newClientPeer()
	}
	m.connMocks[string(connId)].On(methodName, arguments...)
}

func (s *mockApplication) expectConnDisconn(connId wsproxy.ConnectionID) {
	s.on(mockMethodConnect, connId)
	s.on(mockMethodDisconnected, connId)
}

func (m *mockApplication) getCalls(connId wsproxy.ConnectionID) []mock.Call {
	m.connMocksMux.Lock()
	defer m.connMocksMux.Unlock()
	if _, ok := m.connMocks[string(connId)]; !ok {
		return []mock.Call{}
	}
	return m.connMocks[string(connId)].Calls
}

func (s *mockApplication) sendToClient(connId wsproxy.ConnectionID, message messageJSON) error {
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

func parseMessageJSON(value []byte) messageJSON {
	message := map[string]string{}
	unmarshalErr := json.Unmarshal(value, &message)
	if unmarshalErr != nil {
		panic(unmarshalErr)
	}
	return message
}
