package proto

import (
	"errors"
	"fmt"

	"github.com/BarneyLaw/stele-sync/internal/core/classify"
	"github.com/BarneyLaw/stele-sync/internal/core/ids"
)

// Code is a closed submission-rejection code.
type Code string

const (
	// InvalidSubmission is the invalid_submission submission outcome.
	InvalidSubmission Code = "invalid_submission"
	// InvalidOp is the invalid_op submission outcome.
	InvalidOp Code = "invalid_op"
	// InvalidPath is the invalid_path submission outcome.
	InvalidPath Code = "invalid_path"
	// TooLarge is the too_large submission outcome.
	TooLarge Code = "too_large"
	// RateLimited is the rate_limited submission outcome.
	RateLimited Code = "rate_limited"
	// UnknownFile is the unknown_file submission outcome.
	UnknownFile Code = "unknown_file"
	// FileExists is the file_exists submission outcome.
	FileExists Code = "file_exists"
	// FileDeleted is the file_deleted submission outcome.
	FileDeleted Code = "file_deleted"
	// PathTaken is the path_taken submission outcome.
	PathTaken Code = "path_taken"
	// StaleDelete is the stale_delete submission outcome.
	StaleDelete Code = "stale_delete"
	// InvalidBase is the invalid_base submission outcome.
	InvalidBase Code = "invalid_base"
	// ReadOnlyPath is the read_only_path submission outcome.
	ReadOnlyPath Code = "read_only_path"
	// ClassChange is the class_change submission outcome.
	ClassChange Code = "class_change"
	// GroupRequired is the group_required submission outcome.
	GroupRequired Code = "group_required"
	// ClassMismatch is the class_mismatch submission outcome.
	ClassMismatch Code = "class_mismatch"
	// BulkDeleteGuard is the bulk_delete_guard submission outcome.
	BulkDeleteGuard Code = "bulk_delete_guard"
	// RebaseRequired is the rebase_required submission outcome.
	RebaseRequired Code = "rebase_required"
	// WholeDocumentRequired is the whole_document_required submission outcome.
	WholeDocumentRequired Code = "whole_document_required"
	// BlobMissing is the blob_missing submission outcome.
	BlobMissing Code = "blob_missing"
	// Internal is the internal submission outcome.
	Internal Code = "internal"
)

// Codes returns the closed rejection catalogue in stable order.
func Codes() []Code {
	return []Code{InvalidSubmission, InvalidOp, InvalidPath, TooLarge, RateLimited, UnknownFile, FileExists, FileDeleted, PathTaken, StaleDelete, InvalidBase, ReadOnlyPath, ClassChange, GroupRequired, ClassMismatch, BulkDeleteGuard, RebaseRequired, WholeDocumentRequired, BlobMissing, Internal}
}

// QueryCode identifies a failed correlated query.
type QueryCode string

const (
	// ResyncRequired requests a full file listing.
	ResyncRequired QueryCode = "resync_required"
	// InvalidRequest reports a malformed query body.
	InvalidRequest QueryCode = "invalid_request"
	// QueryRateLimited requests a bounded retry delay.
	QueryRateLimited QueryCode = "rate_limited"
	// QueryInternal reports a transient query failure.
	QueryInternal QueryCode = "internal"
)

// QueryCodes returns the query-error catalogue.
func QueryCodes() []QueryCode {
	return []QueryCode{ResyncRequired, InvalidRequest, QueryRateLimited, QueryInternal}
}

// CloseCode is a WebSocket close status.
type CloseCode uint16

const (
	// CloseNormal is a clean close.
	CloseNormal CloseCode = 1000
	// CloseShutdown indicates server shutdown.
	CloseShutdown CloseCode = 1001
	// CloseTooLarge indicates a frame limit violation.
	CloseTooLarge CloseCode = 1009
	// CloseInternal indicates an internal failure.
	CloseInternal CloseCode = 1011
	// CloseProtocol indicates an untrustworthy frame.
	CloseProtocol CloseCode = 4400
	// CloseAuth indicates authentication failure or revocation.
	CloseAuth CloseCode = 4401
	// CloseReplaced indicates another session for this device.
	CloseReplaced CloseCode = 4409
	// CloseUnsupported indicates no shared subprotocol.
	CloseUnsupported CloseCode = 4426
	// CloseSlowConsumer indicates outbound backpressure.
	CloseSlowConsumer CloseCode = 4429
)

// Detail contains structured, nonsecret error context.
type Detail struct {
	Field        string
	Reason       string
	Component    *int
	Class        classify.Class
	Demotion     classify.Demotion
	Limit        int64
	Scope        string
	RetryAfterMS int64
}

// CloseError identifies an unattributable frame. Reason never includes input bytes.
type CloseError struct {
	Code   CloseCode
	Reason string
}

func (e *CloseError) Error() string { return fmt.Sprintf("proto: close %d %s", e.Code, e.Reason) }

// RejectError identifies a submission with a trustworthy correlation identifier.
type RejectError struct {
	ID     ids.ClientOpID
	Code   Code
	Detail Detail
}

func (e *RejectError) Error() string {
	return fmt.Sprintf("proto: reject %s field=%s", e.Code, e.Detail.Field)
}

// QueryError identifies a query with a trustworthy request number.
type QueryError struct {
	Req    Req
	Code   QueryCode
	Detail Detail
}

func (e *QueryError) Error() string {
	return fmt.Sprintf("proto: query %s field=%s", e.Code, e.Detail.Field)
}

// ErrFrameTooLarge indicates that a complete encoded frame exceeds its budget.
var ErrFrameTooLarge = errors.New("proto: frame too large")

// ErrInvalidMessage indicates an invalid locally constructed message.
var ErrInvalidMessage = errors.New("proto: invalid message")
