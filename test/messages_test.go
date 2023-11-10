package test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
	wsproxy "wsproxy/internal"
	"wsproxy/internal/logging"

	"github.com/rs/xid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/suite"
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

func (s *sendMessageTestSuite) TestSendAMessageToApp() {
	ctx, cancel := context.WithTimeout(s.ctx, time.Minute)
	defer cancel()

	s.connIdGenerator = func() wsproxy.ConnectionID {
		return wsproxy.CreateID(ctx)
	}

	client := NewClient(s.wsproxyServer, nil)
	_, err := client.connect(ctx)
	s.NoError(err)

	connId := client.connectionId
	message := "message_" + xid.New().String()

	s.mockApp.on(mockMethodMessageReceived, connId, toWsMessage(message))
	s.mockApp.on(mockMethodDisconnected, connId)

	s.Len(s.mockApp.getCalls(connId), 0)

	err = client.writeMessage(ctx, toWsMessage(message))
	s.NoError(err)

	client.disconnect(ctx)
	<-s.mockApp.connMocks[string(connId)].disconnectNotification

	s.Len(s.mockApp.getCalls(connId), 2)

	callIndex := 0
	call := s.getCall(connId, callIndex)
	s.Equal(mockMethodMessageReceived, call.Method)
	s.assertArguments(&call, toWsMessage(message))

	callIndex++
	call = s.getCall(connId, callIndex)
	s.Equal(mockMethodDisconnected, call.Method)
}

func (s *sendMessageTestSuite) TestReceiveAMessageFromApp() {
	ctx, cancel := context.WithTimeout(s.ctx, time.Minute)
	defer cancel()

	s.connIdGenerator = func() wsproxy.ConnectionID {
		return wsproxy.CreateID(ctx)
	}

	msgFromAppChan := make(chan string)

	client := NewClient(s.wsproxyServer, msgFromAppChan)
	_, err := client.connect(ctx)
	s.NoError(err)

	connId := client.connectionId
	msgToReceive := "message_" + xid.New().String()

	s.mockApp.on(mockMethodDisconnected, connId)

	s.Len(s.mockApp.getCalls(connId), 0)

	err = s.mockApp.sendToClient(connId, toWsMessage(msgToReceive))
	s.NoError(err)

	msgFromApp := <-msgFromAppChan
	s.Equal(msgToReceive, msgFromApp)

	client.disconnect(ctx)
	<-s.mockApp.connMocks[string(connId)].disconnectNotification

	s.Len(s.mockApp.getCalls(connId), 1)

	callIndex := 0
	call := s.getCall(connId, callIndex)
	s.Equal(mockMethodDisconnected, call.Method)
}

func (s *sendMessageTestSuite) testSendReceiveMessagesFromApp(ctx context.Context, logger zerolog.Logger, nrOneWayMessages int) {
	msgFromAppChan := make(chan string, nrOneWayMessages)

	client := NewClient(s.wsproxyServer, msgFromAppChan)
	_, err := client.connect(ctx)
	s.NoError(err)

	connId := client.connectionId

	s.mockApp.on(mockMethodDisconnected, connId)

	s.Len(s.mockApp.getCalls(connId), 0)

	msgsToSend := generateMessages(nrOneWayMessages)
	msgsToReceive := generateMessages(nrOneWayMessages)

	wg := sync.WaitGroup{}
	wg.Add(3 * nrOneWayMessages)

	sendReceiveStart := time.Now()
	go func() {
		start := time.Now()

		for msg := range msgsToReceive {
			err = s.mockApp.sendToClient(connId, toWsMessage(msg))
			wg.Done()
			s.NoError(err)
		}

		logger.Debug().Dur("sending to client", time.Since(start)).Msg(">>>>>>>>>>>>>>> sending to client")
	}()

	go func() {
		start := time.Now()

		for msg := range msgsToSend {
			jsonMsg := toWsMessage(msg)
			s.mockApp.on(mockMethodMessageReceived, connId, jsonMsg)
			err = client.writeMessage(ctx, jsonMsg)
			wg.Done()
			s.NoError(err)
		}

		logger.Debug().Dur("sending to app", time.Since(start)).Msg(">>>>>>>>>>>>>>> sending to app")
	}()

	msgsReceived := map[string]struct{}{}
	go func() {
		start := time.Now()

		for {
			msgFromApp := <-msgFromAppChan
			logger.Debug().Str("msgFromApp", msgFromApp).Msg("client channels message it received")
			msgsReceived[msgFromApp] = struct{}{}
			wg.Done()
			logger.Debug().Dur("receiving from app", time.Since(start)).Msg(">>>>>>>>>>>>>>> receiving from app")
		}

	}()

	wg.Wait()
	sendReceiveDuration := time.Since(sendReceiveStart)

	logger.Info().Dur("sending-receiving", sendReceiveDuration).Msg(">>>>>>>>>>>>>>>>>>>")

	client.disconnect(ctx)
	<-s.mockApp.connMocks[string(connId)].disconnectNotification

	s.Equal(msgsToReceive, msgsReceived)

	s.Len(s.mockApp.getCalls(connId), nrOneWayMessages+1)

	msgsSent := map[string]struct{}{}
	for index := 0; index < nrOneWayMessages+1; index++ {
		call := s.getCall(connId, index)
		if call.Method == mockMethodMessageReceived {
			messageArg := call.Arguments[0]
			msg, ok := messageArg.(messageJSON)
			if !ok {
				panic(fmt.Sprintf("%v (%T) is not a map[string]string", messageArg, messageArg))
			}
			msgsSent[msg["message"]] = struct{}{}
		}
	}

	s.Equal(msgsToSend, msgsSent)
}

func (s *sendMessageTestSuite) TestSendReceiveMessagesFromApp() {
	logger := zerolog.Ctx(s.ctx).With().Str("method", "TestSendReceiveMessagesFromApp").Logger()
	ctx, cancel := context.WithTimeout(s.ctx, time.Minute)
	defer cancel()

	s.connIdGenerator = func() wsproxy.ConnectionID {
		return wsproxy.CreateID(ctx)
	}

	nrOneWayMessages := 50

	s.testSendReceiveMessagesFromApp(ctx, logger, nrOneWayMessages)
}

func (s *sendMessageTestSuite) TestSendReceiveMessagesFromAppMultiClients() {
	logger := zerolog.Ctx(s.ctx).With().Str("method", "TestSendReceiveMessagesFromApp").Logger()
	ctx, cancel := context.WithTimeout(s.ctx, time.Minute)
	defer cancel()

	s.connIdGenerator = func() wsproxy.ConnectionID {
		return wsproxy.CreateID(ctx)
	}

	nrOneWayMessages := 50
	nrClients := 7

	wg := sync.WaitGroup{}
	wg.Add(nrClients)
	for run := 0; run < nrClients; run++ {
		task := run
		go func() {
			start := time.Now()
			s.testSendReceiveMessagesFromApp(ctx, logger, nrOneWayMessages)
			wg.Done()
			logger.Info().Int("taskId", task).Dur("run time", time.Since(start)).Msg(">>>>>>>>>>>>>>>")
		}()
	}
	wg.Wait()
}

func generateMessages(nr int) map[string]struct{} {
	messages := map[string]struct{}{}
	for index := 0; index < nr; index++ {
		msg := fmt.Sprintf("message_%d_%s", index, xid.New().String())
		messages[msg] = struct{}{}
	}
	return messages
}
