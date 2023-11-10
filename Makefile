.PHONY: clean test-single

test-envs = LOG_LEVEL=debug APP_ENV=development

clean:
	go clean -testcache
test-all:
	$(test-envs) go test -v -parallel 1 -timeout 600s ./test/...
test-single:
	#$(test-envs) go test -v -parallel 1 -timeout 10s ./test/... -run '^TestConnectingTestSuite$$' # -testify.m '^TestConnectionID$$'
	$(test-envs) go test -v -parallel 1 -timeout 60s ./test/... -run '^TestSendMessageTestSuite$$' -testify.m '^TestSendReceiveMessagesFromAppMultiClients$$'
