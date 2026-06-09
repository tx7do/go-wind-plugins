module github.com/tx7do/go-wind-plugins/log/cloudwatch

go 1.26.3

require (
	github.com/aws/aws-sdk-go-v2 v1.36.5
	github.com/aws/aws-sdk-go-v2/config v1.29.17
	github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs v1.50.0
	github.com/tx7do/go-wind v0.0.1
)

require (
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.6.10 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.17.70 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.16.32 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.36 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.36 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.12.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.12.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.25.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.30.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.34.0 // indirect
	github.com/aws/smithy-go v1.22.4 // indirect
)

// Redirect the old monolithic genproto to a version where googleapis/rpc
// has been split out, avoiding ambiguous import conflicts.
replace google.golang.org/genproto => google.golang.org/genproto v0.0.0-20260406210006-6f92a3bedf2d

// github.com/lyft/protoc-gen-validate has moved to envoyproxy/protoc-gen-validate.
// The old repository is gone, so we redirect all transitive references.
replace github.com/lyft/protoc-gen-validate => github.com/envoyproxy/protoc-gen-validate v0.10.1
