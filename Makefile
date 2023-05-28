.PHONY: clean test-single

test-envs = LOG_LEVEL=debug APP_ENV=development

clean:
	go clean -testcache
test-single-unit:
	$(test-envs) go test -v -parallel 1 -v -timeout 10s ./test/... -v -run '^TestConnectingTestSuite$$' # -testify.m '^TestDisconnection$$'
