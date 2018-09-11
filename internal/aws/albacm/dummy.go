package albacm

import (
	"github.com/aws/aws-sdk-go/service/acm"
)

// Dummy is a dummy implementation of albacm.ACMWithStatus.
type Dummy struct {
	ACMWithStatus

	outputs output
}

type output map[string]interface{}

func (o output) error(s string) error {
	if v, ok := o[s]; ok && v != nil {
		return v.(error)
	}
	return nil
}

// NewDummy creates a new albacm.Dummy.
func NewDummy() *Dummy {
	d := &Dummy{}
	d.outputs = make(output)
	return d
}

// SetField sets a result field in this Dummy.
func (d *Dummy) SetField(field string, v interface{}) {
	d.outputs[field] = v
}

// ListCertificates is a dummy implementation.
func (d *Dummy) ListCertificates(*acm.ListCertificatesInput) (*acm.ListCertificatesOutput, error) {
	return d.outputs["ListCertificatesOutput"].(*acm.ListCertificatesOutput), d.outputs.error("ListCertificatesError")
}

// ListCertificatesPages is a dummy implementation.
func (d *Dummy) ListCertificatesPages(_ *acm.ListCertificatesInput, fn func(_ *acm.ListCertificatesOutput, _ bool) bool) (error) {
	err := d.outputs.error("ListCertificatesError")
	if err != nil {
		return err
	}

	fn(d.outputs["ListCertificatesOutput"].(*acm.ListCertificatesOutput), true)
	return nil
}
