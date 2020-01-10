package conditions

import (
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/pkg/errors"
)

const (
	FieldHostHeader        = "host-header"
	FieldPathPattern       = "path-pattern"
	FieldHTTPHeader        = "http-header"
	FieldHTTPRequestMethod = "http-request-method"
	FieldQueryString       = "query-string"
	FieldSourceIP          = "source-ip"
)

// Information about a host header condition.
type HostHeaderConditionConfig struct {
	// One or more host names. The maximum size of each name is 128 characters.
	// The comparison is case insensitive. The following wildcard characters are
	// supported: * (matches 0 or more characters) and ? (matches exactly 1 character).
	//
	// If you specify multiple strings, the condition is satisfied if one of the
	// strings matches the host name.
	Values []*string
}

func (c *HostHeaderConditionConfig) validate() error {
	if len(c.Values) == 0 {
		return errors.New("Values cannot be empty")
	}
	return nil
}

// Information about an HTTP header condition.
//
// There is a set of standard HTTP header fields. You can also define custom
// HTTP header fields.
type HttpHeaderConditionConfig struct {
	// The name of the HTTP header field. The maximum size is 40 characters. The
	// header name is case insensitive. The allowed characters are specified by
	// RFC 7230. Wildcards are not supported.
	//
	// You can't use an HTTP header condition to specify the host header. Use HostHeaderConditionConfig
	// to specify a host header condition.
	HttpHeaderName *string

	// One or more strings to compare against the value of the HTTP header. The
	// maximum size of each string is 128 characters. The comparison strings are
	// case insensitive. The following wildcard characters are supported: * (matches
	// 0 or more characters) and ? (matches exactly 1 character).
	//
	// If the same header appears multiple times in the request, we search them
	// in order until a match is found.
	//
	// If you specify multiple strings, the condition is satisfied if one of the
	// strings matches the value of the HTTP header. To require that all of the
	// strings are a match, create one condition per string.
	Values []*string
}

func (c *HttpHeaderConditionConfig) validate() error {
	if len(c.Values) == 0 {
		return errors.New("Values cannot be empty")
	}
	return nil
}

// Information about an HTTP method condition.
//
// HTTP defines a set of request methods, also referred to as HTTP verbs. For
// more information, see the HTTP Method Registry (https://www.iana.org/assignments/http-methods/http-methods.xhtml).
// You can also define custom HTTP methods.
type HttpRequestMethodConditionConfig struct {
	// The name of the request method. The maximum size is 40 characters. The allowed
	// characters are A-Z, hyphen (-), and underscore (_). The comparison is case
	// sensitive. Wildcards are not supported; therefore, the method name must be
	// an exact match.
	//
	// If you specify multiple strings, the condition is satisfied if one of the
	// strings matches the HTTP request method. We recommend that you route GET
	// and HEAD requests in the same way, because the response to a HEAD request
	// may be cached.
	Values []*string
}

func (c *HttpRequestMethodConditionConfig) validate() error {
	if len(c.Values) == 0 {
		return errors.New("Values cannot be empty")
	}
	return nil
}

// Information about a path pattern condition.
type PathPatternConditionConfig struct {
	// One or more path patterns to compare against the request URL. The maximum
	// size of each string is 128 characters. The comparison is case sensitive.
	// The following wildcard characters are supported: * (matches 0 or more characters)
	// and ? (matches exactly 1 character).
	//
	// If you specify multiple strings, the condition is satisfied if one of them
	// matches the request URL. The path pattern is compared only to the path of
	// the URL, not to its query string. To compare against the query string, use
	// QueryStringConditionConfig.
	Values []*string
}

func (c *PathPatternConditionConfig) validate() error {
	if len(c.Values) == 0 {
		return errors.New("Values cannot be empty")
	}
	return nil
}

// Information about a key/value pair.
type QueryStringKeyValuePair struct {
	// The key. You can omit the key.
	Key *string

	// The value.
	Value *string
}

// Information about a query string condition.
//
// The query string component of a URI starts after the first '?' character
// and is terminated by either a '#' character or the end of the URI. A typical
// query string contains key/value pairs separated by '&' characters. The allowed
// characters are specified by RFC 3986. Any character can be percentage encoded.
type QueryStringConditionConfig struct {
	// One or more key/value pairs or values to find in the query string. The maximum
	// size of each string is 128 characters. The comparison is case insensitive.
	// The following wildcard characters are supported: * (matches 0 or more characters)
	// and ? (matches exactly 1 character). To search for a literal '*' or '?' character
	// in a query string, you must escape these characters in Values using a '\'
	// character.
	//
	// If you specify multiple key/value pairs or values, the condition is satisfied
	// if one of them is found in the query string.
	Values []*QueryStringKeyValuePair
}

func (c *QueryStringConditionConfig) validate() error {
	if len(c.Values) == 0 {
		return errors.New("Values cannot be empty")
	}
	return nil
}

// Information about a source IP condition.
//
// You can use this condition to route based on the IP address of the source
// that connects to the load balancer. If a client is behind a proxy, this is
// the IP address of the proxy not the IP address of the client.
type SourceIpConditionConfig struct {
	// One or more source IP addresses, in CIDR format. You can use both IPv4 and
	// IPv6 addresses. Wildcards are not supported.
	//
	// If you specify multiple addresses, the condition is satisfied if the source
	// IP address of the request matches one of the CIDR blocks. This condition
	// is not satisfied by the addresses in the X-Forwarded-For header. To search
	// for addresses in the X-Forwarded-For header, use HttpHeaderConditionConfig.
	Values []*string
}

func (c *SourceIpConditionConfig) validate() error {
	if len(c.Values) == 0 {
		return errors.New("Values cannot be empty")
	}
	return nil
}

// Information about a condition for a rule.
type RuleCondition struct {
	// The field in the HTTP request. The following are the possible values:
	//
	//    * http-header
	//
	//    * http-request-method
	//
	//    * host-header
	//
	//    * path-pattern
	//
	//    * query-string
	//
	//    * source-ip
	Field *string

	// Information for a host header condition. Specify only when Field is host-header.
	HostHeaderConfig *HostHeaderConditionConfig

	// Information for an HTTP header condition. Specify only when Field is http-header.
	HttpHeaderConfig *HttpHeaderConditionConfig

	// Information for an HTTP method condition. Specify only when Field is http-request-method.
	HttpRequestMethodConfig *HttpRequestMethodConditionConfig

	// Information for a path pattern condition. Specify only when Field is path-pattern.
	PathPatternConfig *PathPatternConditionConfig

	// Information for a query string condition. Specify only when Field is query-string.
	QueryStringConfig *QueryStringConditionConfig

	// Information for a source IP condition. Specify only when Field is source-ip.
	SourceIpConfig *SourceIpConditionConfig
}

func (c *RuleCondition) validate() error {
	switch aws.StringValue(c.Field) {
	case FieldHostHeader:
		if c.HostHeaderConfig == nil {
			return errors.New("missing HostHeaderConfig")
		}
		if err := c.HostHeaderConfig.validate(); err != nil {
			return errors.Wrap(err, "invalid HostHeaderConfig")
		}
	case FieldPathPattern:
		if c.PathPatternConfig == nil {
			return errors.New("missing PathPatternConfig")
		}
		if err := c.PathPatternConfig.validate(); err != nil {
			return errors.Wrap(err, "invalid PathPatternConfig")
		}
	case FieldHTTPHeader:
		if c.HttpHeaderConfig == nil {
			return errors.New("missing HttpHeaderConfig")
		}
		if err := c.HttpHeaderConfig.validate(); err != nil {
			return errors.Wrap(err, "invalid HttpHeaderConfig")
		}
	case FieldHTTPRequestMethod:
		if c.HttpRequestMethodConfig == nil {
			return errors.New("missing HttpRequestMethodConfig")
		}
		if err := c.HttpRequestMethodConfig.validate(); err != nil {
			return errors.Wrap(err, "invalid HttpRequestMethodConfig")
		}
	case FieldQueryString:
		if c.QueryStringConfig == nil {
			return errors.New("missing QueryStringConfig")
		}
		if err := c.QueryStringConfig.validate(); err != nil {
			return errors.Wrap(err, "invalid QueryStringConfig")
		}
	case FieldSourceIP:
		if c.SourceIpConfig == nil {
			return errors.New("missing SourceIpConfig")
		}
		if err := c.SourceIpConfig.validate(); err != nil {
			return errors.Wrap(err, "invalid SourceIpConfig")
		}
	}
	return nil
}
