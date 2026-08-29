package model

import (
	"fmt"
	"maps"
	"strconv"
	"time"
)

type ID int32

const IDBits = 32

func ParseID(s string) (ID, error) {
	id, err := strconv.ParseInt(s, 10, IDBits)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("id must be int%d > 0", IDBits)
	}
	return ID(id), nil
}

func (id ID) String() string { return strconv.FormatInt(int64(id), 10) }

type UserID ID

type UserRoles map[string]Role

type User struct {
	ID           UserID    `json:"id,omitempty"`
	Email        string    `json:"email,omitempty" format:"email"`
	PasswordHash string    `json:"-"`
	Name         string    `json:"name,omitempty"`
	CreatedAt    time.Time `json:"created_at,omitzero" format:"date-time"`
	Roles        UserRoles `json:"roles,omitempty" swaggertype:"object,string" example:"123:member"`
}

func (u User) Clone() User {
	u.Roles = maps.Clone(u.Roles)
	return u
}

type TeamID ID

func (id TeamID) String() string { return ID(id).String() }

type Team struct {
	ID        TeamID       `json:"id,omitempty"`
	Name      string       `json:"name,omitempty"`
	CreatedBy UserID       `json:"created_by,omitempty"`
	CreatedAt time.Time    `json:"created_at,omitzero" format:"date-time"`
	Members   []TeamMember `json:"members,omitempty"`
	Tasks     []Task       `json:"tasks,omitempty"`
}

//go:generate enumer -type Role -linecomment -json -sql
type Role uint8

const (
	_          Role = iota
	RoleOwner       // owner
	RoleAdmin       // admin
	RoleMember      // member
)

type TeamMember struct {
	TeamID TeamID `json:"team_id,omitempty"`
	User   User   `json:"user"`
	Role   Role   `json:"role,omitempty" swaggertype:"string"`
}

//go:generate enumer -type Status -linecomment -json
type Status uint8

const (
	StatusUnknown    Status = 0 // unknown
	StatusTodo       Status = 1 // todo
	StatusInProgress Status = 2 // in_progress
	StatusDone       Status = 3 // done
	StatusCancelled  Status = 4 // cancelled
)

type TaskID ID

type Task struct {
	ID          TaskID              `json:"id,omitempty"`
	TeamID      TeamID              `json:"team_id,omitempty"`
	Title       string              `json:"title,omitempty"`
	Description string              `json:"description,omitempty"`
	Status      Status              `json:"status,omitempty" swaggertype:"string" enums:"todo,in_progress,done,cancelled"`
	CreatedBy   UserID              `json:"created_by,omitempty"`
	AssigneeID  Nullable[UserID]    `json:"assignee_id,omitzero" swaggertype:"integer"`
	CreatedAt   time.Time           `json:"created_at,omitzero" format:"date-time"`
	UpdatedAt   time.Time           `json:"updated_at,omitzero" format:"date-time"`
	ClosedAt    Nullable[time.Time] `json:"closed_at,omitzero" swaggertype:"string" format:"date-time"`
	Version     int64               `json:"version,omitempty"`
	History     []TaskHistoryEvent  `json:"history,omitempty"`
	Comments    []TaskComment       `json:"comments,omitempty"`
}

type TaskHistoryEventID ID

type TaskHistoryEvent struct {
	ID        TaskHistoryEventID `json:"id,omitempty"`
	TaskID    TaskID             `json:"task_id,omitempty"`
	ChangedBy UserID             `json:"changed_by,omitempty"`
	Changes   map[string]*Change `json:"changes,omitempty"`
	CreatedAt time.Time          `json:"created_at,omitzero" format:"date-time"`
}

type Change struct {
	Old any `json:"old"`
	New any `json:"new"`
}

type TaskCommentID ID

type TaskComment struct {
	ID        TaskCommentID `json:"id,omitempty"`
	TaskID    TaskID        `json:"task_id,omitempty"`
	UserID    UserID        `json:"user_id,omitempty"`
	Content   string        `json:"content,omitempty"`
	CreatedAt time.Time     `json:"created_at,omitzero" format:"date-time"`
}
