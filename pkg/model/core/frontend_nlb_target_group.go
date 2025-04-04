package core

// FrontendNlbTargetGroupState represents the state of a single ALB Target Type target group with its ALB target
type FrontendNlbTargetGroupState struct {
	Name       string
	ARN        StringToken
	Port       int32
	TargetARN  StringToken
	TargetPort int32
}

// FrontendNlbTargetGroupDesiredState maintains a mapping of target groups targeting ALB
type FrontendNlbTargetGroupDesiredState struct {
	TargetGroups map[string]*FrontendNlbTargetGroupState
}

func NewFrontendNlbTargetGroupDesiredState() *FrontendNlbTargetGroupDesiredState {
	return &FrontendNlbTargetGroupDesiredState{
		TargetGroups: make(map[string]*FrontendNlbTargetGroupState),
	}
}

func (m *FrontendNlbTargetGroupDesiredState) AddTargetGroup(targetGroupName string, targetGroupARN StringToken, targetARN StringToken, port int32, targetPort int32) {
	m.TargetGroups[targetGroupName] = &FrontendNlbTargetGroupState{
		Name:       targetGroupName,
		ARN:        targetGroupARN,
		Port:       port,
		TargetARN:  targetARN,
		TargetPort: targetPort,
	}
}
