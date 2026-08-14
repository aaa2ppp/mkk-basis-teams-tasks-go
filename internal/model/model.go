package model

import "time"

type UserID int32

type User struct {
	UserID
	Email        string
	PasswordHash string
	Name         string
	CreatedAt    time.Time
}

type TeamID int32

type Team struct {
	TeamID
	Name      string
	CreatedBy *User
	CreatedAt time.Time
	Members   []User
}

//go:generate enumer -type TaskStatus -linecomment -json
type TaskStatus uint8

const (
	UnknownStatus    TaskStatus = 0
	OpenStatus       TaskStatus = 1
	InProgressStatus TaskStatus = 2
	InReviewStatus   TaskStatus = 3
	ResolvedStatus   TaskStatus = 4
	ClosedStatus     TaskStatus = 5
)

type TaskID int32

type Task struct {
	TaskID
	TeamID
	Title       string
	Description string
	Status      TaskStatus
	CreatedBy   *User
	Assignee    *User
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ClosedAt    time.Time
	Version     string
	Comments    []TaskComment
	TaskHistory []TaskEvent
}

type TaskEventID int32

type TaskEvent struct {
	TaskEventID
	ChangedBy *User
	Changes   string
	CreatedAt time.Time
}

type TaskCommentID int32

type TaskComment struct {
	TaskCommentID
	TaskID
	User      *User
	Content   string
	CreatedAt time.Time
}

//go:generate enumer -type Role -linecomment -json
type Role uint8

const (
	UnknownRole Role = 0 // unknown
	OwnerRole   Role = 1 // owner
	AdminRole   Role = 2 // admin
	MamberRole  Role = 3 // member
)
