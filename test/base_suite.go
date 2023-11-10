package test

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	wsproxy "wsproxy/internal"
)

type baseTestSuite struct {
	suite.Suite
	wsproxyServer   string
	ctx             context.Context
	wsGateway       *wsproxy.Server
	mockApp         *mockApplication
	connIdGenerator func() wsproxy.ConnectionID
	// Fall-back connection-id in case no generator is specified to be used in strictly sequential test cases
	// testing in isolation the connection setup itself
	nextConnId wsproxy.ConnectionID
}

func NewBaseTestSuite(ctx context.Context) *baseTestSuite {
	return &baseTestSuite{
		ctx: ctx,
	}
}

func (s *baseTestSuite) startMockApp() {
	s.mockApp = newMockApp(func() string {
		return fmt.Sprintf("http://%s", s.wsproxyServer)
	})
	mockAppStartErr := s.mockApp.start()
	if mockAppStartErr != nil {
		panic(mockAppStartErr)
	}
}

func (s *baseTestSuite) SetupSuite() {
	logger := zerolog.Ctx(s.ctx).With().Str("method", "SetupSuite").Logger()
	logger.Info().Msg("BEGIN")

	s.startMockApp()

	server := wsproxy.NewServer(
		s.ctx,
		wsproxy.Config{
			ServerHost:          "localhost",
			ServerPort:          0,
			AppBaseUrl:          fmt.Sprintf("http://%s", s.mockApp.listener.Addr().String()),
			LoadBalancerAddress: "",
		},
		func() wsproxy.ConnectionID {
			if s.connIdGenerator == nil {
				return s.nextConnId
			}
			return s.connIdGenerator()
		},
	)
	s.wsGateway = server

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		err := server.SetupAndStart(func(port int, _ func()) {
			fmt.Fprint(os.Stderr, "WsGateway is ready!\n")
			s.wsproxyServer = fmt.Sprintf("localhost:%d", port)
			wg.Done()
		})
		logger.Warn().Err(err).Msg("error during server start")
	}()
	wg.Wait()
}

func (s *baseTestSuite) TearDownSuite() {
	if s.mockApp != nil {
		s.mockApp.stop()
	}
	if s.wsGateway != nil {
		s.wsGateway.Stop()
	}
}

func (s *baseTestSuite) BeforeTest(suiteName, testName string) {
	logger := s.mockApp.logger.With().Str("method", "BeforeTest").Logger()
	logger.Debug().Msg("BEGIN")
	logger.Debug().Msg("END")
}

func (s *baseTestSuite) getCall(connId wsproxy.ConnectionID, callIndex int) mock.Call {
	calls := s.mockApp.getCalls(connId)
	return calls[callIndex]
}

func (s *baseTestSuite) assertArguments(call *mock.Call, objects ...interface{}) {
	call.Arguments.Assert(s.T(), objects...)
}

func toWsMessage(content string) messageJSON {
	return messageJSON{"message": content}
}
