# CargoShip: Enterprise Data Archiving for AWS

> **Forked from:** [SuitcaseCTL by Duke University](https://gitlab.oit.duke.edu/devil-ops/suitcasectl)  
> **License:** MIT (maintaining original attribution)  
> **Vision:** Next-generation enterprise data archiving optimized for AWS infrastructure

## Executive Summary

CargoShip is an enterprise-grade data archiving solution that transforms large research datasets into optimally-compressed, cost-efficient archives for AWS cloud storage. Built as an evolution of Duke University's SuitcaseCTL, CargoShip adds native AWS integration, intelligent cost optimization, and enterprise observability.

**Key Value Propositions:**
- **3x faster** S3 uploads vs generic cloud tools
- **50% cost reduction** through intelligent storage tiering
- **Enterprise observability** with CloudWatch metrics and X-Ray tracing
- **Compliance-ready** with KMS encryption and audit logging

---

## Project Overview

### What CargoShip Does

CargoShip moves large datasets to AWS with intelligence and efficiency:

1. **Discovery & Analysis**: Scans directories and estimates costs before archiving
2. **Intelligent Packing**: Uses bin-packing algorithms to optimize archive sizes
3. **Parallel Compression**: Creates multiple compressed archives concurrently
4. **Smart Uploading**: Native S3 multipart uploads with adaptive concurrency
5. **Cost Optimization**: Automatic storage class selection and lifecycle management
6. **Enterprise Monitoring**: Complete observability with metrics, tracing, and alerting

### Core Philosophy

**"Ship it smart, ship it fast, ship it cost-effectively"**

- **Intelligence First**: Every decision (storage class, compression, concurrency) is data-driven
- **AWS Native**: Built specifically for AWS, not a generic cloud adapter
- **Enterprise Ready**: Designed for production environments with compliance and monitoring
- **Cost Conscious**: Real-time cost estimation and optimization recommendations

---

## Technical Architecture

### High-Level Architecture

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Data Sources  │    │    CargoShip     │    │   AWS Services  │
│                 │    │     Engine       │    │                 │
│ • File Systems  │───▶│                  │───▶│ • S3 Storage    │
│ • Network Mounts│    │ • Discovery      │    │ • KMS Encryption│
│ • Archives      │    │ • Compression    │    │ • CloudWatch    │
│ • Databases     │    │ • Upload Manager │    │ • X-Ray Tracing │
└─────────────────┘    │ • Cost Optimizer │    │ • Lambda        │
                       └──────────────────┘    └─────────────────┘
```

### Core Components

#### 1. Discovery Engine (`pkg/discovery/`)
```go
type DiscoveryEngine struct {
    scanner     FileScanner
    inventory   InventoryManager
    estimator   CostEstimator
    filters     []Filter
}

// Capabilities:
// - Parallel directory traversal
// - Smart file categorization
// - Deduplication detection
// - Size optimization recommendations
```

#### 2. Compression Manager (`pkg/compression/`)
```go
type CompressionManager struct {
    algorithms  map[string]Compressor
    pools       WorkerPoolManager
    progress    ProgressTracker
}

// Supported formats:
// - tar.zst (default - best performance)
// - tar.gz (compatibility)
// - tar.bz2 (maximum compression)
// - All with optional KMS encryption
```

#### 3. AWS Transport Layer (`pkg/aws/`)
```go
type S3Transporter struct {
    client      *s3.Client
    uploader    *manager.Uploader
    optimizer   StorageOptimizer
    monitor     PerformanceMonitor
}

// Features:
// - Native multipart uploads
// - Adaptive concurrency
// - Storage class optimization
// - Transfer acceleration
// - Cost tracking
```

#### 4. Intelligence Engine (`pkg/intelligence/`)
```go
type IntelligenceEngine struct {
    costCalculator    CostCalculator
    performanceModel  PerformancePredictor
    optimizer        ResourceOptimizer
}

// Capabilities:
// - Real-time cost estimation
// - Performance prediction
// - Resource optimization
// - Storage class recommendations
```

---

## Implementation Roadmap

### Phase 1: Foundation (Weeks 1-4)
**Goal:** Core AWS integration with basic functionality

#### Week 1: Project Bootstrap
- [ ] Repository setup with proper Duke attribution
- [ ] Go module initialization with AWS SDK v2
- [ ] Core interfaces and package structure
- [ ] MIT license compliance documentation

#### Week 2: S3 Transport Implementation
- [ ] Native S3 client with multipart uploads
- [ ] Progress tracking and error handling
- [ ] Basic storage class selection
- [ ] Configuration management

#### Week 3: Cost Intelligence
- [ ] AWS Pricing API integration
- [ ] Real-time cost estimation
- [ ] Storage class optimization
- [ ] Cost reporting and alerts

#### Week 4: CLI Interface
- [ ] Cobra-based command structure
- [ ] Configuration validation
- [ ] Basic upload commands
- [ ] Cost estimation commands

**Deliverables:**
- Working MVP with S3 uploads
- Cost estimation functionality
- Basic CLI interface
- Comprehensive tests

### Phase 2: Performance & Features (Weeks 5-8)
**Goal:** Enterprise-grade performance and advanced features

#### Week 5: Performance Optimization
- [ ] Adaptive concurrency management
- [ ] Bandwidth optimization
- [ ] Memory usage optimization
- [ ] Compression algorithm selection

#### Week 6: Monitoring & Observability
- [ ] CloudWatch metrics integration
- [ ] X-Ray distributed tracing
- [ ] Performance dashboards
- [ ] Alert management

#### Week 7: Security & Compliance
- [ ] KMS encryption integration
- [ ] IAM policy generation
- [ ] Audit logging
- [ ] Compliance reporting

#### Week 8: Integration Testing
- [ ] End-to-end test suite
- [ ] Performance benchmarking
- [ ] Load testing framework
- [ ] Documentation updates

**Deliverables:**
- Production-ready performance
- Complete observability stack
- Security compliance features
- Comprehensive testing

### Phase 3: Enterprise Features (Weeks 9-12)
**Goal:** Advanced enterprise integration and automation

#### Week 9: Serverless Integration
- [ ] Lambda function triggers
- [ ] EventBridge integration
- [ ] Step Functions workflows
- [ ] API Gateway interfaces

#### Week 10: Advanced Monitoring
- [ ] Custom CloudWatch dashboards
- [ ] Automated alerting system
- [ ] Cost anomaly detection
- [ ] Performance trend analysis

#### Week 11: Multi-Region & DR
- [ ] Cross-region replication
- [ ] Disaster recovery planning
- [ ] Regional optimization
- [ ] Failover mechanisms

#### Week 12: Automation & Optimization
- [ ] Intelligent lifecycle policies
- [ ] Automated cost optimization
- [ ] Predictive scaling
- [ ] Resource right-sizing

**Deliverables:**
- Full enterprise feature set
- Automated operations
- Disaster recovery capabilities
- Advanced optimization

### Phase 4: Production Readiness (Weeks 13-16)
**Goal:** Production deployment and ecosystem

#### Week 13-14: Hardening
- [ ] Production error handling
- [ ] Security vulnerability scanning
- [ ] Performance profiling
- [ ] Reliability testing

#### Week 15-16: Release & Documentation
- [ ] Complete documentation
- [ ] Deployment automation
- [ ] CI/CD pipeline
- [ ] Community resources

**Deliverables:**
- Production-ready release
- Complete documentation
- Deployment automation
- Community launch

---

## CLI Interface Design

### Command Structure
```bash
cargoship [global-flags] <command> [command-flags] [args]

# Global flags
--config string     Config file path
--profile string    AWS profile
--region string     AWS region
--verbose          Enable verbose logging
--dry-run          Show what would be done
```

### Core Commands

#### Discovery & Planning
```bash
# Survey data sources
cargoship survey /path/to/data
cargoship survey --format json --output inventory.json /research/datasets

# Cost estimation
cargoship estimate /path/to/data
cargoship estimate --storage-class glacier --show-recommendations /data

# Preview operations
cargoship plan /path/to/data --destination s3://my-bucket/archives
```

#### Archive Operations
```bash
# Basic archiving
cargoship ship /path/to/data --destination s3://my-bucket

# Advanced archiving with options
cargoship ship /research/project-1 \
  --destination s3://research-archives/project-1 \
  --storage-class glacier \
  --compression zstd \
  --encrypt-kms arn:aws:kms:us-east-1:123456789:key/12345678-1234 \
  --max-archive-size 10GB \
  --concurrency 8 \
  --max-monthly-cost 500

# Resume interrupted uploads
cargoship resume --job-id abc123

# Batch operations
cargoship ship-batch --config batch-config.yaml
```

#### Monitoring & Management
```bash
# Status and monitoring
cargoship status
cargoship jobs list
cargoship jobs status --job-id abc123

# Cost management
cargoship costs show
cargoship costs optimize --dry-run
cargoship costs alert --threshold 1000

# Performance monitoring
cargoship metrics
cargoship dashboard create
cargoship alerts configure
```

#### Configuration & Setup
```bash
# Setup and configuration
cargoship configure
cargoship configure aws --profile research
cargoship validate-config

# Storage management
cargoship storage create-bucket --bucket my-archives
cargoship storage setup-lifecycle --bucket my-archives
cargoship storage optimize --bucket my-archives
```

---

## Configuration Management

### Configuration File Structure
```yaml
# ~/.cargoship/config.yaml
aws:
  profile: research
  region: us-east-1
  
storage:
  default_bucket: research-archives
  storage_class: intelligent_tiering
  lifecycle_enabled: true
  replication:
    enabled: true
    destination_bucket: research-archives-backup
    destination_region: us-west-2

compression:
  algorithm: zstd
  level: 3
  max_archive_size: 10GB
  
upload:
  concurrency: 8
  multipart_threshold: 100MB
  multipart_chunk_size: 10MB
  retry_attempts: 3
  
cost_control:
  max_monthly_budget: 1000.00
  alert_threshold: 0.8
  auto_optimize: true
  require_approval_over: 500.00

monitoring:
  cloudwatch_enabled: true
  xray_tracing: true
  custom_dashboard: true
  alerts:
    - type: cost_threshold
      value: 800.00
      sns_topic: arn:aws:sns:us-east-1:123:cost-alerts
    - type: upload_failure
      consecutive_failures: 3
      sns_topic: arn:aws:sns:us-east-1:123:ops-alerts

security:
  kms_key_id: arn:aws:kms:us-east-1:123:key/12345678-1234
  encryption_required: true
  iam_role_arn: arn:aws:iam::123456789:role/CargoShipRole
  
metadata:
  include_checksums: true
  include_timestamps: true
  custom_tags:
    project: research-project-1
    department: data-science
    classification: internal
```

### Environment Variables
```bash
# AWS Configuration
CARGOSHIP_AWS_PROFILE=research
CARGOSHIP_AWS_REGION=us-east-1
CARGOSHIP_S3_BUCKET=research-archives

# Performance Tuning
CARGOSHIP_CONCURRENCY=8
CARGOSHIP_MEMORY_LIMIT=8GB
CARGOSHIP_TEMP_DIR=/tmp/cargoship

# Cost Controls
CARGOSHIP_MAX_MONTHLY_COST=1000
CARGOSHIP_REQUIRE_APPROVAL=true

# Security
CARGOSHIP_KMS_KEY_ID=arn:aws:kms:us-east-1:123:key/12345
CARGOSHIP_ENCRYPTION_REQUIRED=true
```

---

## Cost Optimization Strategy

### Intelligent Storage Class Selection
```go
type StorageClassOptimizer struct {
    accessPatterns map[string]AccessPattern
    costCalculator CostCalculator
}

type OptimizationRule struct {
    Condition   string // "size > 1GB AND access_frequency = rare"
    StorageClass string // "DEEP_ARCHIVE"
    Savings     float64 // Expected monthly savings
}

// Example optimization rules:
var DefaultOptimizationRules = []OptimizationRule{
    {
        Condition:    "size > 100GB AND access_frequency = never",
        StorageClass: "DEEP_ARCHIVE",
        Savings:      0.75, // 75% cost reduction
    },
    {
        Condition:    "retention_days > 90 AND access_frequency = rare",
        StorageClass: "GLACIER",
        Savings:      0.60, // 60% cost reduction
    },
    {
        Condition:    "size > 1GB AND access_frequency = infrequent",
        StorageClass: "STANDARD_IA",
        Savings:      0.40, // 40% cost reduction
    },
}
```

### Cost Estimation Engine
```go
type CostEstimate struct {
    UploadCosts     CostBreakdown `json:"upload_costs"`
    StorageCosts    CostBreakdown `json:"storage_costs"`
    TransferCosts   CostBreakdown `json:"transfer_costs"`
    TotalMonthlyCost float64      `json:"total_monthly_cost"`
    TotalAnnualCost  float64      `json:"total_annual_cost"`
    Recommendations []Recommendation `json:"recommendations"`
}

type CostBreakdown struct {
    Standard      float64 `json:"standard"`
    StandardIA    float64 `json:"standard_ia"`
    Glacier       float64 `json:"glacier"`
    DeepArchive   float64 `json:"deep_archive"`
    Total         float64 `json:"total"`
}

type Recommendation struct {
    Type            string  `json:"type"`
    Description     string  `json:"description"`
    EstimatedSavings float64 `json:"estimated_savings"`
    Confidence      float64 `json:"confidence"`
}
```

---

## Monitoring & Observability

### CloudWatch Metrics
```go
// Custom metrics published to CloudWatch
type Metrics struct {
    // Performance metrics
    UploadDurationSeconds    float64
    CompressionRatio        float64
    ThroughputMBps          float64
    ConcurrentUploads       float64
    
    // Cost metrics
    EstimatedMonthlyCost    float64
    ActualCosts             float64
    CostPerGB               float64
    
    // Reliability metrics
    SuccessRate             float64
    RetryCount              float64
    ErrorRate               float64
    
    // Resource metrics
    MemoryUsageMB           float64
    CPUUtilization          float64
    NetworkUtilization      float64
}
```

### X-Ray Tracing
```go
// Distributed tracing for complex operations
func TraceArchiveOperation(ctx context.Context, operation string, fn func() error) error {
    ctx, segment := xray.BeginSegment(ctx, fmt.Sprintf("cargoship-%s", operation))
    defer segment.Close(nil)
    
    // Add operation metadata
    segment.AddMetadata("operation", map[string]interface{}{
        "type":      operation,
        "timestamp": time.Now(),
        "version":   version.String(),
    })
    
    // Trace sub-operations
    return traceSubOperations(ctx, fn)
}

func traceSubOperations(ctx context.Context, fn func() error) error {
    // Discovery phase
    ctx, discoverySegment := xray.BeginSubsegment(ctx, "discovery")
    // ... discovery work
    discoverySegment.Close(nil)
    
    // Compression phase
    ctx, compressionSegment := xray.BeginSubsegment(ctx, "compression")
    // ... compression work
    compressionSegment.Close(nil)
    
    // Upload phase
    ctx, uploadSegment := xray.BeginSubsegment(ctx, "upload")
    err := fn()
    if err != nil {
        uploadSegment.AddError(err)
    }
    uploadSegment.Close(err)
    
    return err
}
```

### Alerting Strategy
```yaml
# Alert definitions
alerts:
  # Cost alerts
  - name: monthly-cost-threshold
    metric: EstimatedMonthlyCost
    threshold: 1000
    comparison: greater_than
    period: 24h
    evaluation_periods: 1
    
  # Performance alerts  
  - name: upload-performance-degradation
    metric: ThroughputMBps
    threshold: 50
    comparison: less_than
    period: 5m
    evaluation_periods: 3
    
  # Reliability alerts
  - name: high-error-rate
    metric: ErrorRate
    threshold: 0.05  # 5%
    comparison: greater_than
    period: 5m
    evaluation_periods: 2
```

---

## Security & Compliance

### Encryption Strategy
```go
type EncryptionConfig struct {
    Type           EncryptionType `yaml:"type"`
    KMSKeyID       string        `yaml:"kms_key_id"`
    CustomerKey    string        `yaml:"customer_key,omitempty"`
    EncryptionMode EncryptionMode `yaml:"mode"`
}

type EncryptionType string
const (
    EncryptionSSES3  EncryptionType = "SSE-S3"
    EncryptionSSEKMS EncryptionType = "SSE-KMS"
    EncryptionSSEC   EncryptionType = "SSE-C"
)

type EncryptionMode string
const (
    EncryptArchive EncryptionMode = "archive"    // Encrypt entire archive
    EncryptFiles   EncryptionMode = "files"      // Encrypt individual files
    EncryptBoth    EncryptionMode = "both"       // Both levels
)
```

### IAM Policy Templates
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "CargoShipS3Access",
      "Effect": "Allow",
      "Action": [
        "s3:PutObject",
        "s3:PutObjectAcl",
        "s3:GetObject",
        "s3:DeleteObject",
        "s3:ListBucket"
      ],
      "Resource": [
        "arn:aws:s3:::${bucket_name}",
        "arn:aws:s3:::${bucket_name}/*"
      ],
      "Condition": {
        "StringEquals": {
          "s3:x-amz-server-side-encryption": "aws:kms"
        }
      }
    },
    {
      "Sid": "CargoShipKMSAccess",
      "Effect": "Allow",
      "Action": [
        "kms:Encrypt",
        "kms:Decrypt",
        "kms:ReEncrypt*",
        "kms:GenerateDataKey*",
        "kms:DescribeKey"
      ],
      "Resource": "arn:aws:kms:*:*:key/${kms_key_id}"
    },
    {
      "Sid": "CargoShipMonitoring",
      "Effect": "Allow",
      "Action": [
        "cloudwatch:PutMetricData",
        "xray:PutTraceSegments",
        "xray:PutTelemetryRecords"
      ],
      "Resource": "*"
    }
  ]
}
```

### Audit Logging
```go
type AuditLogger struct {
    logger    *slog.Logger
    cloudTrail CloudTrailPublisher
}

type AuditEvent struct {
    Timestamp    time.Time         `json:"timestamp"`
    Operation    string           `json:"operation"`
    User         string           `json:"user"`
    Source       string           `json:"source"`
    Destination  string           `json:"destination"`
    Size         int64            `json:"size"`
    Cost         float64          `json:"cost"`
    Success      bool             `json:"success"`
    Error        string           `json:"error,omitempty"`
    Metadata     map[string]interface{} `json:"metadata"`
}

func (a *AuditLogger) LogArchiveOperation(event AuditEvent) {
    // Log to structured logs
    a.logger.Info("archive operation",
        "operation", event.Operation,
        "user", event.User,
        "size", event.Size,
        "cost", event.Cost,
        "success", event.Success,
    )
    
    // Send to CloudTrail for compliance
    a.cloudTrail.PublishEvent(event)
}
```

---

## Legal Compliance & Attribution

### License File Structure
```
cargoship/
├── LICENSE                    # MIT License for CargoShip
├── NOTICE                     # Attribution and acknowledgments
├── THIRD_PARTY_LICENSES/     # All dependency licenses
│   ├── SUITCASECTL_LICENSE   # Duke University's original MIT license
│   ├── AWS_SDK_LICENSE       # AWS SDK license
│   └── ...                   # Other dependencies
└── ATTRIBUTION.md            # Detailed attribution documentation
```

### LICENSE File
```text
MIT License

Copyright (c) 2024 [Your Name/Organization]

Portions of this software are derived from SuitcaseCTL:
Copyright (c) Duke University
Original source: https://gitlab.oit.duke.edu/devil-ops/suitcasectl

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

### NOTICE File
```text
CargoShip - Enterprise Data Archiving for AWS
Copyright 2024 [Your Name/Organization]

This product includes software developed by Duke University:
  SuitcaseCTL (https://gitlab.oit.duke.edu/devil-ops/suitcasectl)
  Copyright (c) Duke University
  Licensed under MIT License

This product includes software developed at Amazon.com, Inc.:
  AWS SDK for Go v2 (https://github.com/aws/aws-sdk-go-v2)
  Licensed under Apache License 2.0

[Additional attributions for other dependencies...]
```

### ATTRIBUTION.md
```markdown
# CargoShip Attribution and Acknowledgments

## Original Project Acknowledgment

CargoShip is built upon the excellent foundation provided by **SuitcaseCTL**, 
developed by Duke University's DevOps team. We are grateful for their 
innovative work in research data archiving and their decision to release 
the project under the MIT License.

**Original Project:**
- **Name:** SuitcaseCTL
- **Author:** Duke University
- **License:** MIT License
- **Repository:** https://gitlab.oit.duke.edu/devil-ops/suitcasectl
- **Original Copyright:** Copyright (c) Duke University

## Key Concepts Inherited

The following core concepts and architectural patterns were inherited from 
SuitcaseCTL and adapted for AWS-specific optimization:

1. **Porter Pattern:** Central orchestration of archiving operations
2. **Inventory System:** File discovery and metadata collection
3. **Suitcase Metaphor:** Breaking large datasets into manageable chunks
4. **Pluggable Transports:** Modular transport layer architecture
5. **Travel Agent Pattern:** Cloud-based orchestration service

## Evolution and Enhancements

While CargoShip builds on SuitcaseCTL's foundation, it represents a 
significant evolution with:

- Complete rewrite for AWS-native performance
- Advanced cost optimization and intelligence
- Enterprise observability and monitoring
- Production-ready security and compliance features
- Modern Go practices and architecture

## Dependencies and Third-Party Software

CargoShip includes and depends on numerous open-source projects. See the
`THIRD_PARTY_LICENSES/` directory for complete license information.

## Community and Contributions

We encourage collaboration between the CargoShip and SuitcaseCTL communities.
Features and improvements developed in CargoShip that are generally applicable
may be contributed back to the upstream SuitcaseCTL project where appropriate.
```

---

## Performance Targets & Success Metrics

### Performance Benchmarks
```go
type PerformanceBenchmarks struct {
    // Upload performance (vs rclone baseline)
    UploadSpeedImprovement    float64 // Target: 3x faster
    ConcurrencyEfficiency     float64 // Target: 95% resource utilization
    MemoryOverhead           float64 // Target: <2% vs original
    
    // Cost optimization
    CostReduction            float64 // Target: 50% through intelligent tiering
    OptimizationAccuracy     float64 // Target: 90% correct storage class selection
    
    // Reliability
    UploadSuccessRate        float64 // Target: 99.9%
    RetrySuccessRate         float64 // Target: 95% recovery from transient failures
    
    // Observability
    MetricsLatency           time.Duration // Target: <1s metric publication
    TracingOverhead          float64       // Target: <1% performance impact
}
```

### Key Performance Indicators (KPIs)
```yaml
performance_kpis:
  upload_speed:
    target: 200  # MB/s average
    measurement: "Average throughput across all storage classes"
    
  cost_optimization:
    target: 50   # Percent cost reduction
    measurement: "Monthly cost vs naive STANDARD storage"
    
  reliability:
    target: 99.9 # Percent success rate
    measurement: "Successful uploads / total upload attempts"
    
  user_experience:
    target: 30   # Seconds to first progress update
    measurement: "Time from command start to first progress indicator"

operational_kpis:
  deployment_frequency:
    target: "weekly"
    measurement: "Production deployment cadence"
    
  mean_time_to_recovery:
    target: 15   # Minutes
    measurement: "Average time to resolve production issues"
    
  error_budget:
    target: 0.1  # Percent acceptable error rate
    measurement: "Monthly error rate allowance"
```

---

## Testing Strategy

### Unit Testing
```go
// Example test structure
func TestS3Transporter_Upload(t *testing.T) {
    tests := []struct {
        name           string
        archive        Archive
        config         S3Config
        expectError    bool
        expectedMetrics UploadMetrics
    }{
        {
            name: "standard upload success",
            archive: Archive{
                Name: "test-archive",
                Size: 100 * MB,
                StorageClass: "STANDARD",
            },
            config: S3Config{
                Bucket: "test-bucket",
                Region: "us-east-1",
            },
            expectError: false,
            expectedMetrics: UploadMetrics{
                ThroughputMBps: 50.0,
                SuccessRate:    1.0,
            },
        },
        // Additional test cases...
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

### Integration Testing
```go
// Integration test with real AWS services
func TestE2EArchiveWorkflow(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test")
    }
    
    // Setup test environment
    testBucket := createTestBucket(t)
    defer cleanupTestBucket(testBucket)
    
    // Create test data
    testData := generateTestData(t, 1*GB)
    defer cleanupTestData(testData)
    
    // Execute full workflow
    config := &Config{
        AWS: AWSConfig{
            S3: S3Config{
                Bucket: testBucket,
                Region: "us-east-1",
            },
        },
    }
    
    engine := NewArchiveEngine(config)
    result, err := engine.ArchiveDirectory(context.Background(), testData.Path)
    
    // Verify results
    require.NoError(t, err)
    assert.Greater(t, result.CompressionRatio, 0.5)
    assert.Less(t, result.UploadDuration, 5*time.Minute)
    
    // Verify S3 objects exist
    objects := listS3Objects(t, testBucket)
    assert.NotEmpty(t, objects)
}
```

### Performance Testing
```go
// Load testing framework
func TestLoadPerformance(t *testing.T) {
    config := LoadTestConfig{
        ConcurrentUploads: 10,
        ArchiveSize:      100 * MB,
        Duration:         5 * time.Minute,
        TargetRegion:     "us-east-1",
    }
    
    results := RunLoadTest(t, config)
    
    // Performance assertions
    avgThroughput := results.AverageThroughput()
    assert.Greater(t, avgThroughput, 50.0, "Average throughput should be > 50 MB/s")
    
    successRate := results.SuccessRate()
    assert.Greater(t, successRate, 0.99, "Success rate should be > 99%")
    
    p95Latency := results.P95Latency()
    assert.Less(t, p95Latency, 30*time.Second, "P95 latency should be < 30s")
}
```

---

## Deployment & Operations

### Container Strategy
```dockerfile
# Multi-stage build for optimal size
FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o cargoship ./cmd/cargoship

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/cargoship .

ENTRYPOINT ["./cargoship"]
```

### Kubernetes Deployment
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: cargoship
  namespace: data-ops
spec:
  replicas: 3
  selector:
    matchLabels:
      app: cargoship
  template:
    metadata:
      labels:
        app: cargoship
    spec:
      serviceAccountName: cargoship
      containers:
      - name: cargoship
        image: cargoship:latest
        env:
        - name: AWS_REGION
          value: "us-east-1"
        - name: CARGOSHIP_CONFIG
          value: "/etc/cargoship/config.yaml"
        volumeMounts:
        - name: config
          mountPath: /etc/cargoship
        resources:
          limits:
            memory: "8Gi"
            cpu: "4"
          requests:
            memory: "4Gi"
            cpu: "2"
      volumes:
      - name: config
        configMap:
          name: cargoship-config
```

### Terraform Infrastructure
```hcl
# S3 bucket with intelligent tiering
resource "aws_s3_bucket" "archives" {
  bucket = var.archive_bucket_name
}

resource "aws_s3_bucket_intelligent_tiering_configuration" "archives" {
  bucket = aws_s3_bucket.archives.id
  name   = "entire-bucket"
  
  status = "Enabled"
  
  optional_fields = [
    "BucketKeyStatus",
    "AccessPointArn"
  ]
}

# KMS key for encryption
resource "aws_kms_key" "cargoship" {
  description = "CargoShip archive encryption key"
  
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "Enable IAM User Permissions"
        Effect = "Allow"
        Principal = {
          AWS = "arn:aws:iam::${data.aws_caller_identity.current.account_id}:root"
        }
        Action   = "kms:*"
        Resource = "*"
      }
    ]
  })
}

# IAM role for CargoShip
resource "aws_iam_role" "cargoship" {
  name = "CargoShipRole"
  
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "ec2.amazonaws.com"
        }
      }
    ]
  })
}

resource "aws_iam_role_policy" "cargoship" {
  name = "CargoShipPolicy"
  role = aws_iam_role.cargoship.id
  
  policy = templatefile("${path.module}/cargoship-policy.json", {
    bucket_arn = aws_s3_bucket.archives.arn
    kms_key_arn = aws_kms_key.cargoship.arn
  })
}
```

---

## Success Criteria & Launch Plan

### Pre-Launch Checklist

#### Technical Readiness
- [ ] All Phase 1-4 deliverables completed
- [ ] Performance benchmarks achieved
- [ ] Security audit passed
- [ ] Load testing completed
- [ ] Documentation comprehensive

#### Legal & Compliance
- [ ] MIT license compliance verified
- [ ] Duke University attribution complete
- [ ] Third-party license audit complete
- [ ] Security compliance review passed

#### Operational Readiness
- [ ] Monitoring dashboards configured
- [ ] Alerting system tested
- [ ] Deployment automation verified
- [ ] Incident response procedures documented

### Launch Strategy

#### Phase 1: Private Beta (Weeks 13-14)
- Limited release to 5-10 friendly users
- Focus on real-world usage validation
- Gather performance and usability feedback
- Refine documentation and procedures

#### Phase 2: Public Beta (Weeks 15-16)
- Open source release on GitHub
- Community announcement
- Docker Hub and package registry publication
- Initial community building

#### Phase 3: General Availability (Week 17+)
- v1.0.0 release with stability guarantees
- Production support and SLA
- Enterprise features and support tiers
- Conference presentations and articles

### Success Metrics (6-month targets)

#### Adoption Metrics
- **GitHub Stars:** 500+
- **Downloads:** 10,000+
- **Active Users:** 100+
- **Enterprise Customers:** 10+

#### Performance Metrics
- **Average Upload Speed:** 200 MB/s
- **Cost Reduction:** 50% vs baseline
- **Reliability:** 99.9% success rate
- **User Satisfaction:** 4.5/5 rating

#### Community Metrics
- **Contributors:** 25+
- **Issues Resolved:** 90% within 1 week
- **Documentation Rating:** 4.0/5
- **Community Growth:** 20% monthly

---

## Conclusion

CargoShip represents a significant evolution from SuitcaseCTL, maintaining the excellent foundation while adding enterprise-grade AWS optimization. The project plan ensures proper attribution to Duke University while creating a distinct, valuable tool for the enterprise data archiving market.

**Key Differentiators:**
- **AWS-Native Performance:** 3x faster uploads through native SDK optimization
- **Intelligent Cost Management:** 50% cost reduction through automated optimization
- **Enterprise Observability:** Complete monitoring and tracing stack
- **Production Ready:** Built for enterprise scale and reliability

**Legal Compliance:** Full MIT license compliance with proper attribution maintains the open-source spirit while enabling commercial use and distribution.

This plan provides a clear roadmap from concept to production-ready enterprise tool, with measurable success criteria and comprehensive technical architecture.