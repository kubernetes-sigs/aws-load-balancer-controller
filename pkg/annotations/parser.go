package annotations

import (
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"strconv"
	"strings"
)

type ParseOptions struct {
	// perform an exact match without looking for prefix
	exact bool
	// alternative prefixes to lookup
	alternativePrefixes []string
}

type ParseOption func(opts *ParseOptions)

func WithExact() ParseOption {
	return func(opts *ParseOptions) {
		opts.exact = true
	}
}

func WithAlternativePrefixes(prefixes ...string) ParseOption {
	return func(opts *ParseOptions) {
		opts.alternativePrefixes = append(opts.alternativePrefixes, prefixes...)
	}
}

// Parser is responsible for loading annotations into structured objects.
// It accepts an list of Object annotations and will search through them until desired annotation is found.
type Parser interface {
	// ParseStringAnnotation parses annotation into string value
	// returns whether annotation exists.
	ParseStringAnnotation(annotation string, value *string, annotations map[string]string, opts ...ParseOption) bool

	// ParseBoolAnnotation parses annotation into bool value
	// returns whether annotation exists and error if any
	ParseBoolAnnotation(annotation string, value *bool, annotations map[string]string, opts ...ParseOption) (bool, error)

	// ParseInt64Annotation parses annotation into int64 value,
	// returns whether annotation exists and parser error if any.
	ParseInt64Annotation(annotation string, value *int64, annotations map[string]string, opts ...ParseOption) (bool, error)

	// ParseStringSliceAnnotation parses comma separated values from the annotation into string slice
	// returns true if the annotation exists
	ParseStringSliceAnnotation(annotation string, value *[]string, annotations map[string]string, opts ...ParseOption) bool

	// ParseJSONAnnotation parses json value into the given interface
	// returns true if the annotation exists and parser error if any
	ParseJSONAnnotation(annotation string, value interface{}, annotations map[string]string, opts ...ParseOption) (bool, error)

	// ParseStringMapAnnotation parses comma separated key=value pairs into a map
	// returns true if the annotation exists
	ParseStringMapAnnotation(annotation string, value *map[string]string, annotations map[string]string, opts ...ParseOption) (bool, error)
}

// NewSuffixAnnotationParser returns new suffixAnnotationParser based on specified prefix.
func NewSuffixAnnotationParser(annotationPrefix string) *suffixAnnotationParser {
	return &suffixAnnotationParser{
		annotationPrefix: annotationPrefix,
	}
}

var _ Parser = (*suffixAnnotationParser)(nil)

// suffixAnnotationParser is an Parser implementation that identify annotation by an configurable prefix and suffix.
// suppose annotationPrefix is "alb.ingress.kubernetes.io", when called with annotation "my-annotation", it will
// actually look for annotation "alb.ingress.kubernetes.io/my-annotation" on objects.
type suffixAnnotationParser struct {
	annotationPrefix string
}

func (p *suffixAnnotationParser) ParseStringAnnotation(annotation string, value *string, annotations map[string]string, opts ...ParseOption) bool {
	ret, _ := p.parseStringAnnotation(annotation, value, annotations, opts...)
	return ret
}

func (p *suffixAnnotationParser) ParseBoolAnnotation(annotation string, value *bool, annotations map[string]string, opts ...ParseOption) (bool, error) {
	raw := ""
	exists, matchedKey := p.parseStringAnnotation(annotation, &raw, annotations, opts...)
	if !exists {
		return false, nil
	}
	val, err := strconv.ParseBool(raw)
	if err != nil {
		return true, errors.Wrapf(err, "failed to parse bool annotation, %v: %v", matchedKey, raw)
	}
	*value = val
	return true, nil
}

func (p *suffixAnnotationParser) ParseInt64Annotation(annotation string, value *int64, annotations map[string]string, opts ...ParseOption) (bool, error) {
	raw := ""
	exists, matchedKey := p.parseStringAnnotation(annotation, &raw, annotations, opts...)
	if !exists {
		return false, nil
	}
	i, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return true, errors.Wrapf(err, "failed to parse int64 annotation, %v: %v", matchedKey, raw)
	}
	*value = i
	return true, nil
}

func (p *suffixAnnotationParser) ParseStringSliceAnnotation(annotation string, value *[]string, annotations map[string]string, opts ...ParseOption) bool {
	raw := ""
	if exists, _ := p.parseStringAnnotation(annotation, &raw, annotations, opts...); !exists {
		return false
	}
	*value = splitCommaSeparatedString(raw)
	return true
}

func (p *suffixAnnotationParser) ParseJSONAnnotation(annotation string, value interface{}, annotations map[string]string, opts ...ParseOption) (bool, error) {
	raw := ""
	exists, matchedKey := p.parseStringAnnotation(annotation, &raw, annotations, opts...)
	if !exists {
		return false, nil
	}
	if err := json.Unmarshal([]byte(raw), value); err != nil {
		return true, errors.Wrapf(err, "failed to parse json annotation, %v: %v", matchedKey, raw)
	}
	return true, nil
}

func (p *suffixAnnotationParser) ParseStringMapAnnotation(annotation string, value *map[string]string, annotations map[string]string, opts ...ParseOption) (bool, error) {
	raw := ""
	exists, matchedKey := p.parseStringAnnotation(annotation, &raw, annotations, opts...)
	if !exists {
		return false, nil
	}
	rawKVPairs := splitCommaSeparatedString(raw)
	keyValues := make(map[string]string)
	for _, kvPair := range rawKVPairs {
		parts := strings.SplitN(kvPair, "=", 2)
		if len(parts) != 2 {
			return false, errors.Errorf("failed to parse stringMap annotation, %v: %v", matchedKey, raw)
		}
		key := parts[0]
		value := parts[1]
		if len(key) == 0 {
			return false, errors.Errorf("failed to parse stringMap annotation, %v: %v", matchedKey, raw)
		}
		keyValues[key] = value
	}
	if value != nil {
		*value = keyValues
	}
	return true, nil
}

func (p *suffixAnnotationParser) parseStringAnnotation(annotation string, value *string, annotations map[string]string, opts ...ParseOption) (bool, string) {
	keys := p.buildAnnotationKeys(annotation, opts...)
	for _, key := range keys {
		if raw, ok := annotations[key]; ok {
			*value = raw
			return true, key
		}
	}
	return false, ""
}

// buildAnnotationKey returns list of full annotation keys based on suffix and parse options
func (p *suffixAnnotationParser) buildAnnotationKeys(suffix string, opts ...ParseOption) []string {
	keys := []string{}
	parseOpts := ParseOptions{}
	for _, opt := range opts {
		opt(&parseOpts)
	}
	if parseOpts.exact {
		keys = append(keys, suffix)
	} else {
		keys = append(keys, fmt.Sprintf("%v/%v", p.annotationPrefix, suffix))
		for _, pfx := range parseOpts.alternativePrefixes {
			keys = append(keys, fmt.Sprintf("%v/%v", pfx, suffix))
		}
	}
	return keys
}

func splitCommaSeparatedString(commaSeparatedString string) []string {
	var result []string
	parts := strings.Split(commaSeparatedString, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if len(part) == 0 {
			continue
		}
		result = append(result, part)
	}
	return result
}
