package test

import (
	"context"
	"net/http"
	"testing"
	"time"
	wsproxy "wsproxy/internal"
	"wsproxy/internal/logging"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/suite"
	"nhooyr.io/websocket"
)

type connectingTestSuite struct {
	*baseTestSuite
}

func TestConnectingTestSuite(t *testing.T) {
	logger := logging.Get().Level(zerolog.DebugLevel).With().Str("unit", "TestConnectingTestSuite").Logger()
	ctx := logger.WithContext(context.Background())
	suite.Run(
		t,
		&connectingTestSuite{
			baseTestSuite: NewBaseTestSuite(ctx),
		},
	)
}

func (s *connectingTestSuite) BeforeTest(suiteName string, testName string) {
	s.baseTestSuite.BeforeTest(suiteName, testName)
	logger := s.mockApp.logger.With().Str("method", "BeforeTest").Logger()
	logger.Debug().Msg("BEGIN")
	s.mockApp.recordConnectionFromClient = true
	logger.Debug().Msg("END")
}

func (s *connectingTestSuite) TestConnectionID() {
	ctx, cancel := context.WithTimeout(s.ctx, time.Minute)
	defer cancel()

	connId := wsproxy.CreateID(ctx)
	s.nextConnId = connId

	client := NewClient(s.wsproxyServer, nil)

	message := toWsMessage("hi")

	s.mockApp.expectConnDisconn(connId)
	s.mockApp.on(mockMethodMessageReceived, connId, message)

	s.Len(s.mockApp.getCalls(connId), 0)

	_, err := client.connect(ctx)
	s.NoError(err)
	if err != nil {
		return
	}
	defer func() {
		client.disconnect(ctx)
		<-s.mockApp.connMocks[string(connId)].disconnectNotification
	}()

	callIndex := 0
	s.Len(s.mockApp.getCalls(connId), callIndex+1)
	call := s.getCall(connId, callIndex)

	err = client.writeMessage(ctx, message)
	s.NoError(err)
	s.Equal(mockMethodConnect, call.Method)
}

func (s *connectingTestSuite) TestConnectingWithInvalidCredentials() {
	ctx, cancel := context.WithTimeout(s.ctx, time.Minute)
	defer cancel()

	connId := wsproxy.CreateID(ctx)
	s.nextConnId = connId

	client := NewClient(s.wsproxyServer, nil)

	response, wsConnectErr := client.connect(ctx, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{badCredential},
		},
	})
	s.Error(wsConnectErr)
	s.Equal(response.StatusCode, http.StatusUnauthorized)

	s.Len(s.mockApp.getCalls(connId), 0)
}

func (s *connectingTestSuite) TestDisconnection() {
	ctx, cancel := context.WithTimeout(s.ctx, time.Minute)
	defer cancel()

	connId := wsproxy.CreateID(ctx)
	s.nextConnId = connId

	client := NewClient(s.wsproxyServer, nil)

	s.mockApp.expectConnDisconn(connId)

	s.Len(s.mockApp.getCalls(connId), 0)

	_, err := client.connect(ctx)
	s.NoError(err)
	if err != nil {
		return
	}

	s.Equal(connId, client.connectionId)

	callIndex := 0
	s.Len(s.mockApp.getCalls(connId), callIndex+1)
	call := s.getCall(connId, callIndex)
	s.Equal(mockMethodConnect, call.Method)

	client.disconnect(ctx)
	<-s.mockApp.connMocks[string(connId)].disconnectNotification

	callIndex++
	call = s.getCall(connId, callIndex)
	s.Len(s.mockApp.getCalls(connId), callIndex+1)
	s.Equal(mockMethodDisconnected, call.Method)
}
