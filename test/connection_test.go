package test

import (
	"context"
	"net/http"
	"testing"
	"time"
	wsgw "websocket-gateway/internal"
	"websocket-gateway/internal/logging"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/suite"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

type connectingTestSuite struct {
	*baseTestSuite
}

func TestConnectingTestSuite(t *testing.T) {
	rootLogger := logging.Get().Level(zerolog.DebugLevel)
	suite.Run(
		t,
		&connectingTestSuite{
			baseTestSuite: NewBaseTestSuite(logging.CreateUnitLogger(rootLogger, "TestConnectingTestSuite")),
		},
	)
}

func (s *connectingTestSuite) BeforeTest(suiteName string, testName string) {
	s.mockApp.resetCalls()
}

func (s *connectingTestSuite) TestConnectionID() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	s.nextConnId = wsgw.CreateID(s.logger)

	s.mockApp.mockMock.On(mockMethodConnecting, wsgw.ConnectionIDHeaderKey, string(s.nextConnId))
	s.mockApp.mockMock.On(mockMethodDisconnected, wsgw.ConnectionIDHeaderKey, string(s.nextConnId))

	s.Len(s.mockApp.mockMock.Calls, 0)

	c, _, err := s.connectToWsgw(ctx, defaultDialOptions)
	s.NoError(err)
	if err != nil {
		return
	}
	defer func() {
		c.Close(websocket.StatusNormalClosure, "we're done")
		<-s.mockApp.mockMock.disconnectListener
	}()

	callIndex := 0
	s.Len(s.mockApp.mockMock.Calls, callIndex+1)
	call := s.getCall((callIndex))

	err = wsjson.Write(ctx, c, "hi")
	s.NoError(err)
	s.Equal(mockMethodConnecting, call.Method)
	s.assertArguments(call, wsgw.ConnectionIDHeaderKey, string(s.nextConnId))
	s.Len(s.mockApp.mockMock.Calls, 1)
}

func (s *connectingTestSuite) TestConnectingWithInvalidCredentials() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	s.nextConnId = wsgw.CreateID(s.logger)

	_, response, _ := s.connectToWsgw(ctx, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{badCredential},
		},
	})
	s.Equal(response.StatusCode, 401)

	s.Len(s.mockApp.mockMock.Calls, 0)
}

func (s *connectingTestSuite) TestDisconnection() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	s.nextConnId = wsgw.CreateID(s.logger)

	s.mockApp.mockMock.On(mockMethodConnecting, wsgw.ConnectionIDHeaderKey, string(s.nextConnId))
	s.mockApp.mockMock.On(mockMethodDisconnected, wsgw.ConnectionIDHeaderKey, string(s.nextConnId))

	s.Len(s.mockApp.mockMock.Calls, 0)

	c, _, err := s.connectToWsgw(ctx, defaultDialOptions)
	s.NoError(err)
	if err != nil {
		return
	}

	callIndex := 0
	s.Len(s.mockApp.mockMock.Calls, callIndex+1)
	call := s.getCall((callIndex))
	s.Equal(mockMethodConnecting, call.Method)
	s.assertArguments(call, wsgw.ConnectionIDHeaderKey, string(s.nextConnId))

	c.Close(websocket.StatusNormalClosure, "we're done")
	<-s.mockApp.mockMock.disconnectListener

	callIndex++
	call = s.getCall((callIndex))
	s.Len(s.mockApp.mockMock.Calls, callIndex+1)
	s.Equal(mockMethodDisconnected, call.Method)
	s.assertArguments(call, wsgw.ConnectionIDHeaderKey, string(s.nextConnId))
}
