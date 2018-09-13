package sg

// LbAttachment represents the desired state of SecurityGroups attached to Lb
type LbAttachment struct {
	GroupIDs []string
	LbArn    string
}

// LbAttachmentController controls the LbAttachment
type LbAttachmentController interface {
	Reconcile(*LbAttachment) error
	Delete(*LbAttachment) error
}
