package test

import (
	"context"
	"testing"
	"time"
	wsproxy "wsproxy/internal"
	"wsproxy/internal/logging"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/suite"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

type sendMessageTestSuite struct {
	*baseTestSuite
}

func TestSendMessageTestSuite(t *testing.T) {
	logger := logging.Get().Level(zerolog.DebugLevel).With().Str("unit", "TestSendMessageTestSuite").Logger()
	ctx := logger.WithContext(context.Background())
	suite.Run(
		t,
		&sendMessageTestSuite{
			baseTestSuite: NewBaseTestSuite(ctx),
		},
	)
}

func (s *sendMessageTestSuite) BeforeTest(suiteName string, testName string) {
	s.mockApp.resetCalls()
}

func (s *sendMessageTestSuite) TestSendMessageToApp() {
	ctx, cancel := context.WithTimeout(s.ctx, time.Minute)
	defer cancel()

	s.nextConnId = wsproxy.CreateID(ctx)
	message := "RMDmpVU4pLMvZbZyMQix8nedQfWgSCoX04+Wu3ZBkis="

	s.mockApp.mockMock.On(mockMethodConnect, wsproxy.ConnectionIDHeaderKey, string(s.nextConnId))
	s.mockApp.mockMock.On(mockMethodMessageReceived, wsproxy.ConnectionIDHeaderKey, string(s.nextConnId), toWsMessage(message))
	s.mockApp.mockMock.On(mockMethodDisconnected, wsproxy.ConnectionIDHeaderKey, string(s.nextConnId))

	s.Len(s.mockApp.mockMock.Calls, 0)

	c, _, err := s.connectToWsproxy(ctx, defaultDialOptions)
	s.NoError(err)

	err = wsjson.Write(ctx, c, toWsMessage(message))
	s.NoError(err)

	c.Close(websocket.StatusNormalClosure, "we're done")
	<-s.mockApp.mockMock.disconnectNotification

	s.Len(s.mockApp.mockMock.Calls, 3)

	callIndex := 0
	call := s.getCall(callIndex)
	s.Equal(mockMethodConnect, call.Method)
	s.assertArguments(call, wsproxy.ConnectionIDHeaderKey, string(s.nextConnId))

	callIndex++
	call = s.getCall(callIndex)
	s.Equal(mockMethodMessageReceived, call.Method)
	s.assertArguments(call, wsproxy.ConnectionIDHeaderKey, string(s.nextConnId), toWsMessage(message))

	callIndex++
	call = s.getCall(callIndex)
	s.Equal(mockMethodDisconnected, call.Method)
	s.assertArguments(call, wsproxy.ConnectionIDHeaderKey, string(s.nextConnId))
}
