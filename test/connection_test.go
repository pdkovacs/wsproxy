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
	"nhooyr.io/websocket/wsjson"
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
	s.mockApp.resetCalls()
}

func (s *connectingTestSuite) TestConnectionID() {
	ctx, cancel := context.WithTimeout(s.ctx, time.Minute)
	defer cancel()

	s.nextConnId = wsproxy.CreateID(ctx)

	message := toWsMessage("hi")

	s.mockApp.mockMock.On(mockMethodConnect, wsproxy.ConnectionIDHeaderKey, string(s.nextConnId))
	s.mockApp.mockMock.On(mockMethodMessageReceived, wsproxy.ConnectionIDHeaderKey, string(s.nextConnId), message)
	s.mockApp.mockMock.On(mockMethodDisconnected, wsproxy.ConnectionIDHeaderKey, string(s.nextConnId))

	s.Len(s.mockApp.mockMock.Calls, 0)

	c, _, err := s.connectToWsproxy(ctx, defaultDialOptions)
	s.NoError(err)
	if err != nil {
		return
	}
	defer func() {
		c.Close(websocket.StatusNormalClosure, "we're done")
		<-s.mockApp.mockMock.disconnectNotification
	}()

	callIndex := 0
	s.Len(s.mockApp.mockMock.Calls, callIndex+1)
	call := s.getCall((callIndex))

	err = wsjson.Write(ctx, c, message)
	s.NoError(err)
	s.Equal(mockMethodConnect, call.Method)
	s.assertArguments(call, wsproxy.ConnectionIDHeaderKey, string(s.nextConnId))
	s.Len(s.mockApp.mockMock.Calls, callIndex+1)
}

func (s *connectingTestSuite) TestConnectingWithInvalidCredentials() {
	ctx, cancel := context.WithTimeout(s.ctx, time.Minute)
	defer cancel()

	s.nextConnId = wsproxy.CreateID(ctx)

	_, response, wsConnectErr := s.connectToWsproxy(ctx, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{badCredential},
		},
	})
	s.Error(wsConnectErr)
	s.Equal(response.StatusCode, http.StatusUnauthorized)

	s.Len(s.mockApp.mockMock.Calls, 0)
}

func (s *connectingTestSuite) TestDisconnection() {
	ctx, cancel := context.WithTimeout(s.ctx, time.Minute)
	defer cancel()

	s.nextConnId = wsproxy.CreateID(ctx)

	s.mockApp.mockMock.On(mockMethodConnect, wsproxy.ConnectionIDHeaderKey, string(s.nextConnId))
	s.mockApp.mockMock.On(mockMethodDisconnected, wsproxy.ConnectionIDHeaderKey, string(s.nextConnId))

	s.Len(s.mockApp.mockMock.Calls, 0)

	c, _, err := s.connectToWsproxy(ctx, defaultDialOptions)
	s.NoError(err)
	if err != nil {
		return
	}

	callIndex := 0
	s.Len(s.mockApp.mockMock.Calls, callIndex+1)
	call := s.getCall((callIndex))
	s.Equal(mockMethodConnect, call.Method)
	s.assertArguments(call, wsproxy.ConnectionIDHeaderKey, string(s.nextConnId))

	c.Close(websocket.StatusNormalClosure, "we're done")
	<-s.mockApp.mockMock.disconnectNotification

	callIndex++
	call = s.getCall((callIndex))
	s.Len(s.mockApp.mockMock.Calls, callIndex+1)
	s.Equal(mockMethodDisconnected, call.Method)
	s.assertArguments(call, wsproxy.ConnectionIDHeaderKey, string(s.nextConnId))
}
