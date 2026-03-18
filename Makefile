.PHONY: codegen codegen-events codegen-lambda codegen-openapi build-openapi-docs test-codegen build-push-lambda-images build-push-persistence-writer

.PHONY: analyze-ui analyze-setup

ANALYZE_DIR ?= analyze
ANALYZE_PORT ?= 9090

analyze-setup:
	cd $(ANALYZE_DIR) && pip install -r requirements.txt

analyze-ui: analyze-setup
	cd $(ANALYZE_DIR) && streamlit run app.py --server.port $(ANALYZE_PORT)

codegen: codegen-events codegen-lambda codegen-openapi

codegen-events:
	go generate ./shared/domain/event/...

codegen-lambda:
	go run ./tools/cmd/gen-lambda-handlers/main.go

# 1) 구조만 generated.json 2) enrichment와 병합 → openapi.json 3) API Gateway용 openapi.api-gateway.json
codegen-openapi:
	@mkdir -p infra/openapi infra/terraform
	node scripts/gen-openapi.js
	node scripts/merge-openapi.js
	OPENAPI_FOR=apigateway node scripts/gen-openapi.js

# Redoc HTML from OpenAPI spec (events.json → openapi.json). Output: infra/openapi/dist/index.html
# Run before terraform apply to upload docs to S3.
build-openapi-docs: codegen-openapi
	@mkdir -p infra/openapi/dist
	npx -y @redocly/cli build-docs infra/openapi/openapi.json -o infra/openapi/dist/index.html

#
# Build and push all Lambda container images to ECR.
# Uses:
# - infra/terraform/lambdas.json (keys = lambda_key)
# - ECR repo naming: ${PROJECT}-${lambda_key}
# Tag suffix는 현재 스크립트에서 사용하지 않지만, 추후 확장용으로 남겨둠.
#
LAMBDA_IMAGE_TAG_SUFFIX ?= dev-local
PROJECT ?= go_samams
AWS_REGION ?= ap-northeast-2

build-push-lambda-images:
	aws ecr get-login-password --region $(AWS_REGION) \
  | docker login \
    --username AWS \
    --password-stdin "$$(aws sts get-caller-identity --query Account --output text).dkr.ecr.$(AWS_REGION).amazonaws.com"
	@LAMBDA_KEYS=$$(node -e 'const fs=require("fs");const p="infra/terraform/lambdas.json";const j=JSON.parse(fs.readFileSync(p,"utf8"));for(const k of Object.keys(j)){if(k!=="_comment")console.log(k)}'); \
	for key in $$LAMBDA_KEYS; do \
		echo "=== Building and pushing lambda image for $$key (project=$(PROJECT), region=$(AWS_REGION))"; \
		bash ./scripts/build_and_push_lambda.sh $$key $(PROJECT) $(AWS_REGION) $(LAMBDA_IMAGE_TAG_SUFFIX); \
	done

# Persistence writer: SQS → S3. Not in events.json; ECR repo go_samams-persistence-writer.
build-push-persistence-writer:
	aws ecr get-login-password --region $(AWS_REGION) \
  | docker login \
    --username AWS \
    --password-stdin "$$(aws sts get-caller-identity --query Account --output text).dkr.ecr.$(AWS_REGION).amazonaws.com"
	bash ./scripts/build_and_push_lambda.sh persistence-writer $(PROJECT) $(AWS_REGION) $(LAMBDA_IMAGE_TAG_SUFFIX)

test-codegen: codegen
	@echo "--- checking event.generated.go ---"
	@test -f shared/domain/event/event.generated.go || (echo "missing shared/domain/event/event.generated.go" && exit 1)
	@grep -q 'TypeTaskCreated' shared/domain/event/event.generated.go || (echo "event.generated.go missing expected constant" && exit 1)
	@echo "--- checking lambda handlers (SSOT: events.json with handler) ---"
	@test -f server/cmd/task/created/main.go || (echo "missing server/cmd/task/created/main.go" && exit 1)
	@test -f server/cmd/task/status/updated/main.go || (echo "missing server/cmd/task/status/updated/main.go" && exit 1)
	@grep -q 'task.NewService' server/cmd/task/created/main.go || (echo "task/created/main.go missing expected call" && exit 1)
	@grep -q 'ContextPlanner' server/cmd/task/created/main.go || (echo "task/created/main.go missing ContextPlanner" && exit 1)
	@echo "--- checking generated lambdas.json ---"
	@grep -q 'task-created' infra/terraform/lambdas.json || (echo "lambdas.json missing task-created" && exit 1)
	@grep -q 'task-status-updated' infra/terraform/lambdas.json || (echo "lambdas.json missing task-status-updated" && exit 1)
	@echo "--- building generated cmd ---"
	@cd server && go build ./cmd/task/created/... ./cmd/task/status/updated/... || (echo "server build failed" && exit 1)
	@echo "codegen test OK"
