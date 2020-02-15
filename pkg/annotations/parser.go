package annotations

import (
	"fmt"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/algorithm"
	"strconv"
)

// Parser is responsible for loading annotations into structured objects.
// It accepts an list of Object annotations and will search through them until desired annotation is found.
type Parser interface {
	// ParseStringAnnotation parses annotation into string value
	// returns whether annotation exists.
	ParseStringAnnotation(annotation string, value *string, annotations ...map[string]string) bool

	// ParseInt64Annotation parses annotation into int64 value,
	// returns whether annotation exists and parser error if any.
	ParseInt64Annotation(suffix string, value *int64, annotations ...map[string]string) (bool, error)
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

func (p *suffixAnnotationParser) ParseStringAnnotation(suffix string, value *string, annotations ...map[string]string) bool {
	key := p.buildAnnotationKey(suffix)
	raw, ok := algorithm.MapFindFirst(key, annotations...)
	if !ok {
		return false
	}
	*value = raw
	return true
}

func (p *suffixAnnotationParser) ParseInt64Annotation(suffix string, value *int64, annotations ...map[string]string) (bool, error) {
	key := p.buildAnnotationKey(suffix)
	raw, ok := algorithm.MapFindFirst(key, annotations...)
	if !ok {
		return false, nil
	}
	i, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return true, errors.Wrapf(err, "failed to parse annotation, %v: %v", key, raw)
	}
	*value = i
	return true, nil
}

// buildAnnotationKey returns full annotation key based on suffix
func (p *suffixAnnotationParser) buildAnnotationKey(suffix string) string {
	return fmt.Sprintf("%v/%v", p.annotationPrefix, suffix)
}
