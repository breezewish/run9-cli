package api

import "time"

type MembershipRole string

type OrgKind string

type BoxState string

type SnapState string

type MeView struct {
	UserID       string    `json:"user_id"`
	PrimaryEmail string    `json:"primary_email"`
	DisplayName  string    `json:"display_name,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type OrgView struct {
	OrgID       string         `json:"org_id"`
	DisplayName string         `json:"display_name"`
	Kind        OrgKind        `json:"kind"`
	Role        MembershipRole `json:"role"`
	CreatedBy   string         `json:"created_by"`
	CreatedAt   time.Time      `json:"created_at"`
}

type CurrentOrgIdentityView struct {
	User     MeView  `json:"user"`
	Org      OrgView `json:"org"`
	AuthKind string  `json:"auth_kind"`
}

type BoxView struct {
	BoxID               string            `json:"box_id"`
	OrgID               string            `json:"org_id"`
	Creator             string            `json:"creator"`
	CreatedAt           time.Time         `json:"created_at"`
	Name                string            `json:"name,omitempty"`
	Labels              map[string]string `json:"labels,omitempty"`
	State               BoxState          `json:"state"`
	Reason              string            `json:"reason,omitempty"`
	BoxSnapID           string            `json:"box_snap_id"`
	DesiredShape        string            `json:"desired_shape"`
	CurrentRuntimeShape string            `json:"current_runtime_shape,omitempty"`
	PendingShapeChange  bool              `json:"pending_shape_change"`
}

type SnapView struct {
	SnapID              string    `json:"snap_id"`
	OrgID               string    `json:"org_id"`
	Creator             string    `json:"creator"`
	CreatedAt           time.Time `json:"created_at"`
	State               SnapState `json:"state"`
	Reason              string    `json:"reason,omitempty"`
	ParentChain         []string  `json:"parent_chain,omitempty"`
	SourceImageRef      string    `json:"source_image_ref,omitempty"`
	SourceImageDigest   string    `json:"source_image_digest,omitempty"`
	SourceImagePlatform string    `json:"source_image_platform,omitempty"`
	Attached            bool      `json:"attached"`
	AttachedBoxID       string    `json:"attached_box_id,omitempty"`
}

type RuntimeRequestView struct {
	RuntimeRequestID string `json:"runtime_request_id"`
	State            string `json:"state"`
	SessionID        string `json:"session_id,omitempty"`
	HostID           string `json:"host_id,omitempty"`
}

type CreateBoxRequest struct {
	DesiredShape   string            `json:"desired_shape"`
	Name           string            `json:"name,omitempty"`
	Labels         map[string]string `json:"labels,omitempty"`
	SourceSnapID   string            `json:"source_snap_id,omitempty"`
	SourceImageRef string            `json:"source_image_ref,omitempty"`
}

type ImportSnapRequest struct {
	ImageRef string `json:"image_ref"`
}

type ExecBoxRequest struct {
	DeadlineAt   time.Time         `json:"deadline_at"`
	Command      []string          `json:"command"`
	EnvOverrides map[string]string `json:"env_overrides,omitempty"`
	User         string            `json:"user,omitempty"`
	Workdir      string            `json:"workdir,omitempty"`
}

type ExecStreamEvent struct {
	Type          string `json:"type"`
	Data          []byte `json:"data,omitempty"`
	ExitCode      int32  `json:"exit_code,omitempty"`
	FailureReason string `json:"failure_reason,omitempty"`
	CancelReason  string `json:"cancel_reason,omitempty"`
}
