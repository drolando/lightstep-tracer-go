package lightstep

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"path"
	"strings"
	"time" // N.B.(jmacd): Do not use google.golang.org/glog in this package.

	"github.com/opentracing/opentracing-go"
	"google.golang.org/grpc"
)

// Default Option values.
const (
	DefaultCollectorPath     = "/_rpc/v1/reports/binary"
	DefaultPlainPort         = 80
	DefaultSecurePort        = 443
	DefaultGRPCCollectorHost = "collector-grpc.lightstep.com"

	DefaultMaxReportingPeriod = 2500 * time.Millisecond
	DefaultMinReportingPeriod = 500 * time.Millisecond
	DefaultMaxSpans           = 1000
	DefaultReportTimeout      = 30 * time.Second
	DefaultReconnectPeriod    = 5 * time.Minute

	DefaultMaxLogKeyLen   = 256
	DefaultMaxLogValueLen = 1024
	DefaultMaxLogsPerSpan = 500

	DefaultGRPCMaxCallSendMsgSizeBytes = math.MaxInt32
)

// Tag and Tracer Attribute keys.
const (
	ParentSpanGUIDKey = "parent_span_guid" // ParentSpanGUIDKey is the tag key used to record the relationship between child and parent spans.
	ComponentNameKey  = "lightstep.component_name"
	GUIDKey           = "lightstep.guid" // <- runtime guid, not span guid
	HostnameKey       = "lightstep.hostname"
	CommandLineKey    = "lightstep.command_line"

	TracerPlatformKey        = "lightstep.tracer_platform"
	TracerPlatformValue      = "go"
	TracerPlatformVersionKey = "lightstep.tracer_platform_version"
	TracerVersionKey         = "lightstep.tracer_version" // Note: TracerVersionValue is generated from ./VERSION
)

const (
	secureScheme    = "https"
	plaintextScheme = "http"
)

// Validation Errors
var (
	errInvalidGUIDKey = fmt.Errorf("Options invalid: setting the %v tag is no longer supported", GUIDKey)
)

// A SpanRecorder handles all of the `RawSpan` data generated via an
// associated `Tracer` instance.
type SpanRecorder interface {
	RecordSpan(RawSpan)
}

// Endpoint describes a collector or web API host/port and whether or
// not to use plaintext communication.
type Endpoint struct {
	Scheme           string `yaml:"scheme" json:"scheme" usage:"scheme to use for the endpoint, defaults to appropriate one if no custom one is required"`
	Host             string `yaml:"host" json:"host" usage:"host on which the endpoint is running"`
	Port             int    `yaml:"port" json:"port" usage:"port on which the endpoint is listening"`
	Plaintext        bool   `yaml:"plaintext" json:"plaintext" usage:"whether or not to encrypt data send to the endpoint"`
	CustomCACertFile string `yaml:"custom_ca_cert_file" json:"custom_ca_cert_file" usage:"path to a custom CA cert file, defaults to system defined certs if omitted"`
}

// HostPort use SocketAddress instead.
// DEPRECATED
func (e Endpoint) HostPort() string {
	return e.SocketAddress()
}

// SocketAddress returns an address suitable for dialing grpc connections
func (e Endpoint) SocketAddress() string {
	return fmt.Sprintf("%s:%d", e.Host, e.Port)
}

// URL returns an address suitable for dialing thrift connections
func (e Endpoint) URL() string {
	return fmt.Sprintf("%s%s", e.urlWithoutPath(), DefaultCollectorPath)
}

// urlWithoutPath returns an address suitable for grpc connections if a custom scheme is provided
func (e Endpoint) urlWithoutPath() string {
	return fmt.Sprintf("%s://%s", e.scheme(), e.SocketAddress())
}

func (e Endpoint) scheme() string {
	if len(e.Scheme) > 0 {
		return e.Scheme
	}

	if e.Plaintext {
		return plaintextScheme
	}

	return secureScheme
}

// Options control how the LightStep Tracer behaves.
type Options struct {
	// AccessToken is the unique API key for your LightStep project.  It is
	// available on your account page at https://app.lightstep.com/account
	AccessToken string `yaml:"access_token" usage:"access token for reporting to LightStep"`

	// Collector is the host, port, and plaintext option to use
	// for the collector.
	Collector Endpoint `yaml:"collector"`

	// Tags are arbitrary key-value pairs that apply to all spans generated by
	// this Tracer.
	Tags opentracing.Tags

	// LightStep is the host, port, and plaintext option to use
	// for the LightStep web API.
	LightStepAPI Endpoint `yaml:"lightstep_api"`

	// MaxBufferedSpans is the maximum number of spans that will be buffered
	// before sending them to a collector.
	MaxBufferedSpans int `yaml:"max_buffered_spans"`

	// MaxLogKeyLen is the maximum allowable size (in characters) of an
	// OpenTracing logging key. Longer keys are truncated.
	MaxLogKeyLen int `yaml:"max_log_key_len"`

	// MaxLogValueLen is the maximum allowable size (in characters) of an
	// OpenTracing logging value. Longer values are truncated. Only applies to
	// variable-length value types (strings, interface{}, etc).
	MaxLogValueLen int `yaml:"max_log_value_len"`

	// MaxLogsPerSpan limits the number of logs in a single span.
	MaxLogsPerSpan int `yaml:"max_logs_per_span"`

	// GRPCMaxCallSendMsgSizeBytes limits the size in bytes of grpc messages
	// sent by a client.
	GRPCMaxCallSendMsgSizeBytes int `yaml:"grpc_max_call_send_msg_size_bytes"`

	// ReportingPeriod is the maximum duration of time between sending spans
	// to a collector.  If zero, the default will be used.
	ReportingPeriod time.Duration `yaml:"reporting_period"`

	// MinReportingPeriod is the minimum duration of time between sending spans
	// to a collector.  If zero, the default will be used. It is strongly
	// recommended to use the default.
	MinReportingPeriod time.Duration `yaml:"min_reporting_period"`

	ReportTimeout time.Duration `yaml:"report_timeout"`

	// DropSpanLogs turns log events on all Spans into no-ops.
	DropSpanLogs bool `yaml:"drop_span_logs"`

	// DEPRECATED: The LightStep library prints the first error to stdout by default.
	// See the documentation on the SetGlobalEventHandler function for guidance on
	// how to integrate tracer diagnostics with your application's logging and
	// metrics systems.
	Verbose bool `yaml:"verbose"`

	// Force the use of a specific transport protocol. If multiple are set to true,
	// the following order is used to select for the first option: http, grpc.
	// If none are set to true, HTTP is defaulted to.
	UseHttp bool `yaml:"use_http"`
	UseGRPC bool `yaml:"usegrpc"`

	// CustomCollector allows customizing the Protobuf transport.
	// This is an advanced feature that avoids reconnect logic.
	CustomCollector Collector `yaml:"-" json:"-"`

	ReconnectPeriod time.Duration `yaml:"reconnect_period"`

	// DialOptions allows customizing the grpc dial options passed to the grpc.Dial(...) call.
	// This is an advanced feature added to allow for a custom balancer or middleware.
	// It can be safely ignored if you have no custom dialing requirements.
	// If UseGRPC is not set, these dial options are ignored.
	DialOptions []grpc.DialOption `yaml:"-" json:"-"`

	// A hook for receiving finished span events
	Recorder SpanRecorder `yaml:"-" json:"-"`

	// For testing purposes only
	ConnFactory ConnectorFactory `yaml:"-" json:"-"`

	// Enable LightStep Meta Event Logging
	MetaEventReportingEnabled bool `yaml:"meta_event_reporting_enabled" json:"meta_event_reporting_enabled"`
}

// Initialize validates options, and sets default values for unset options.
// This is called automatically when creating a new Tracer.
func (opts *Options) Initialize() error {
	err := opts.Validate()
	if err != nil {
		return err
	}

	// Note: opts is a copy of the user's data, ok to modify.
	if opts.MaxBufferedSpans == 0 {
		opts.MaxBufferedSpans = DefaultMaxSpans
	}
	if opts.MaxLogKeyLen == 0 {
		opts.MaxLogKeyLen = DefaultMaxLogKeyLen
	}
	if opts.MaxLogValueLen == 0 {
		opts.MaxLogValueLen = DefaultMaxLogValueLen
	}
	if opts.MaxLogsPerSpan == 0 {
		opts.MaxLogsPerSpan = DefaultMaxLogsPerSpan
	}
	if opts.GRPCMaxCallSendMsgSizeBytes == 0 {
		opts.GRPCMaxCallSendMsgSizeBytes = DefaultGRPCMaxCallSendMsgSizeBytes
	}
	if opts.ReportingPeriod == 0 {
		opts.ReportingPeriod = DefaultMaxReportingPeriod
	}
	if opts.MinReportingPeriod == 0 {
		opts.MinReportingPeriod = DefaultMinReportingPeriod
	}
	if opts.ReportTimeout == 0 {
		opts.ReportTimeout = DefaultReportTimeout
	}
	if opts.ReconnectPeriod == 0 {
		opts.ReconnectPeriod = DefaultReconnectPeriod
	}
	if opts.Tags == nil {
		opts.Tags = map[string]interface{}{}
	}

	// Set some default attributes if not found in options
	if _, found := opts.Tags[ComponentNameKey]; !found {
		opts.Tags[ComponentNameKey] = path.Base(os.Args[0])
	}
	if _, found := opts.Tags[HostnameKey]; !found {
		hostname, _ := os.Hostname()
		opts.Tags[HostnameKey] = hostname
	}
	if _, found := opts.Tags[CommandLineKey]; !found {
		opts.Tags[CommandLineKey] = strings.Join(os.Args, " ")
	}

	opts.ReconnectPeriod = time.Duration(float64(opts.ReconnectPeriod) * (1 + 0.2*rand.Float64()))

	if opts.Collector.Host == "" {
		opts.Collector.Host = DefaultGRPCCollectorHost
	}

	if opts.Collector.Port <= 0 {
		if opts.Collector.Plaintext {
			opts.Collector.Port = DefaultPlainPort
		} else {
			opts.Collector.Port = DefaultSecurePort
		}
	}

	return nil
}

// Validate checks that all required fields are set, and no options are incorrectly
// configured.
func (opts *Options) Validate() error {
	if _, found := opts.Tags[GUIDKey]; found {
		return errInvalidGUIDKey
	}

	if len(opts.Collector.CustomCACertFile) != 0 {
		if _, err := os.Stat(opts.Collector.CustomCACertFile); os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// SetSpanID is a opentracing.StartSpanOption that sets an
// explicit SpanID.  It must be used in conjunction with
// SetTraceID or the result is undefined.
type SetSpanID uint64

// Apply satisfies the StartSpanOption interface.
func (sid SetSpanID) Apply(sso *opentracing.StartSpanOptions) {}
func (sid SetSpanID) applyLS(sso *startSpanOptions) {
	sso.SetSpanID = uint64(sid)
}

// SetTraceID is an opentracing.StartSpanOption that sets an
// explicit TraceID.  It must be used in order to set an
// explicit SpanID or ParentSpanID.  If a ChildOf or
// FollowsFrom span relation is also set in the start options,
// it will override this value.
type SetTraceID uint64

// Apply satisfies the StartSpanOption interface.
func (sid SetTraceID) Apply(sso *opentracing.StartSpanOptions) {}
func (sid SetTraceID) applyLS(sso *startSpanOptions) {
	sso.SetTraceID = uint64(sid)
}

// SetParentSpanID is an opentracing.StartSpanOption that sets
// an explicit parent SpanID.  It must be used in conjunction
// with SetTraceID or the result is undefined.  If the value
// is zero, it will be disregarded.  If a ChildOf or
// FollowsFrom span relation is also set in the start options,
// it will override this value.
type SetParentSpanID uint64

// Apply satisfies the StartSpanOption interface.
func (sid SetParentSpanID) Apply(sso *opentracing.StartSpanOptions) {}
func (sid SetParentSpanID) applyLS(sso *startSpanOptions) {
	sso.SetParentSpanID = uint64(sid)
}

// lightStepStartSpanOption is used to identify lightstep-specific Span options.
type lightStepStartSpanOption interface {
	applyLS(*startSpanOptions)
}

type startSpanOptions struct {
	Options opentracing.StartSpanOptions

	// Options to explicitly set span_id, trace_id,
	// parent_span_id, expected to be used when exporting spans
	// from another system into LightStep via opentracing APIs.
	SetSpanID       uint64
	SetParentSpanID uint64
	SetTraceID      uint64
}

func newStartSpanOptions(sso []opentracing.StartSpanOption) startSpanOptions {
	opts := startSpanOptions{}
	for _, o := range sso {
		switch o := o.(type) {
		case lightStepStartSpanOption:
			o.applyLS(&opts)
		default:
			o.Apply(&opts.Options)
		}
	}
	return opts
}
