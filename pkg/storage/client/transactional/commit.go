package transactional

import (
	"fmt"

	"github.com/fluxcd/go-git-providers/validation"
)

// Commit describes a result of a transaction.
type Commit interface {
	// GetAuthor describes the author of this commit.
	// +required
	GetAuthor() CommitAuthor
	// GetMessage describes the change in this commit.
	// +required
	GetMessage() CommitMessage
	// Validate validates that all required fields are set, and given data is valid.
	Validate() error
}

type CommitAuthor interface {
	// GetName describes the author's name (e.g. as per git config)
	// +required
	GetName() string
	// GetEmail describes the author's email (e.g. as per git config).
	// It is optional generally, but might be required by some specific
	// implementations.
	// +optional
	GetEmail() string
	// The String() method must return a (ideally both human- and machine-
	// readable) concatenated string including the name and email (if
	// applicable) of the author.
	fmt.Stringer
}

type CommitMessage interface {
	// GetTitle describes the change concisely, so it can be used e.g. as
	// a commit message or PR title. Certain implementations might enforce
	// character limits on this string.
	// +required
	GetTitle() string
	// GetDescription contains optional extra, more detailed information
	// about the change.
	// +optional
	GetDescription() string
	// The String() method must return a (ideally both human- and machine-
	// readable) concatenated string including the title and description
	// (if applicable) of the author.
	fmt.Stringer
}

// GenericCommitResult implements Commit.
var _ Commit = GenericCommit{}

// GenericCommit implements Commit.
type GenericCommit struct {
	// GetAuthor describes the author of this commit.
	// +required
	Author CommitAuthor
	// GetMessage describes the change in this commit.
	// +required
	Message CommitMessage
}

func (r GenericCommit) GetAuthor() CommitAuthor   { return r.Author }
func (r GenericCommit) GetMessage() CommitMessage { return r.Message }

func (r GenericCommit) Validate() error {
	v := validation.New("GenericCommit")
	if len(r.Author.GetName()) == 0 {
		v.Required("Author.GetName")
	}
	if len(r.Message.GetTitle()) == 0 {
		v.Required("Message.GetTitle")
	}
	return v.Error()
}

// GenericCommitAuthor implements CommitAuthor.
var _ CommitAuthor = GenericCommitAuthor{}

// GenericCommit implements Commit.
type GenericCommitAuthor struct {
	// Name describes the author's name (as per git config)
	// +required
	Name string
	// Email describes the author's email (as per git config)
	// +optional
	Email string
}

func (r GenericCommitAuthor) GetName() string  { return r.Name }
func (r GenericCommitAuthor) GetEmail() string { return r.Email }

func (r GenericCommitAuthor) String() string {
	if len(r.Email) != 0 {
		return fmt.Sprintf("%s <%s>", r.Name, r.Email)
	}
	return r.Name
}

// GenericCommitMessage implements CommitMessage.
var _ CommitMessage = GenericCommitMessage{}

// GenericCommitMessage implements CommitMessage.
type GenericCommitMessage struct {
	// Title describes the change concisely, so it can be used e.g. as
	// a commit message or PR title. Certain implementations might enforce
	// character limits on this string.
	// +required
	Title string
	// Description contains optional extra, more detailed information
	// about the change.
	// +optional
	Description string
}

func (r GenericCommitMessage) GetTitle() string       { return r.Title }
func (r GenericCommitMessage) GetDescription() string { return r.Description }

func (r GenericCommitMessage) String() string {
	if len(r.Description) != 0 {
		return fmt.Sprintf("%s\n\n%s", r.Title, r.Description)
	}
	return r.Title
}
