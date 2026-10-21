package repo

import (
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrNotFound            = errors.New("запись не найдена")
	ErrAlreadyRegistered   = errors.New("учётная запись уже зарегистрирована")
	ErrLoginTaken          = errors.New("логин уже занят")
	ErrEmployeeNumberTaken = errors.New("табельный номер уже занят")
	ErrEmployeeInactive    = errors.New("сотрудник деактивирован")
	ErrIdempotencyConflict = errors.New("операция с таким Idempotency-Key уже выполняется")
	ErrActivePaperQRExists = errors.New("у сотрудника уже есть активный бумажный QR-токен")
	ErrQRTokenExists       = errors.New("QR-токен с таким значением уже существует")
	ErrInvalidTransition   = errors.New("недопустимый переход отметки")
	ErrOpenShiftExists     = errors.New("у сотрудника уже есть открытая смена")
	ErrScheduleOverlap     = errors.New("график пересекается по срокам с существующим")
	ErrNameTaken           = errors.New("название уже занято")
	ErrReferenceNotFound   = errors.New("запись справочника не найдена")
)

type AuthUser struct {
	ID                       string
	EmployeeID               *string
	Role                     string
	Login                    string
	PasswordHash             string
	FailedAttempts           int
	IsLocked                 bool
	AuthBlocked              bool
	IsMainAdmin              bool
	RegistrationResetAllowed bool
}

type Profile struct {
	ID             string  `json:"id"`
	Role           string  `json:"role"`
	Login          string  `json:"login"`
	EmployeeID     *string `json:"employee_id"`
	FullName       *string `json:"full_name"`
	Position       *string `json:"position"`
	EmployeeNumber *string `json:"employee_number"`
}

type User struct {
	ID             string     `json:"id"`
	Role           string     `json:"role"`
	Login          string     `json:"login"`
	EmployeeID     *string    `json:"employee_id"`
	IsLocked       bool       `json:"is_locked"`
	AuthBlocked    bool       `json:"auth_blocked"`
	IsMainAdmin    bool       `json:"is_main_admin"`
	RegisteredAt   *time.Time `json:"registered_at"`
	LastLoginAt    *time.Time `json:"last_login_at"`
	FullName       *string    `json:"full_name,omitempty"`
	EmployeeNumber *string    `json:"employee_number,omitempty"`
}

type Employee struct {
	ID             string  `json:"id"`
	EmployeeNumber string  `json:"employee_number"`
	FullName       string  `json:"full_name"`
	LastName       *string `json:"last_name"`
	FirstName      *string `json:"first_name"`
	MiddleName     *string `json:"middle_name"`
	Email          *string `json:"email"`
	Phone          *string `json:"phone"`
	Position       *string `json:"position"`
	DepartmentID   *string `json:"department_id"`
	PositionID     *string `json:"position_id"`
	IsActive       bool    `json:"is_active"`
	HiredAt        *string `json:"hired_at"`
	FiredAt        *string `json:"fired_at"`
}

type EmployeeInput struct {
	EmployeeNumber string  `json:"employee_number"`
	FullName       string  `json:"full_name"`
	LastName       string  `json:"last_name"`
	FirstName      string  `json:"first_name"`
	MiddleName     string  `json:"middle_name"`
	Email          *string `json:"email"`
	Phone          *string `json:"phone"`
	DepartmentID   *string `json:"department_id"`
	PositionID     *string `json:"position_id"`
	IsActive       *bool   `json:"is_active"`
	HiredAt        *string `json:"hired_at"`
	FiredAt        *string `json:"fired_at"`
}

type Department struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsActive  bool   `json:"is_active"`
	CreatedAt string `json:"created_at"`
}

type DepartmentInput struct {
	Name     string `json:"name"`
	IsActive *bool  `json:"is_active"`
}

type Position struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsActive  bool   `json:"is_active"`
	CreatedAt string `json:"created_at"`
}

type PositionInput struct {
	Name     string `json:"name"`
	IsActive *bool  `json:"is_active"`
}

type QRToken struct {
	ID            string
	EmployeeID    string
	Type          string
	Token         string
	Payload       []byte
	Signature     string
	ValidFrom     *string
	ValidUntil    *string
	Status        string
	RevokedReason *string
	EmployeeOK    bool
	EmployeeNum   string
	FullName      string
	Position      *string
	Department    *string
	Phone         *string
	Email         *string
}

type Mark struct {
	ID                 string     `json:"id"`
	EmployeeID         string     `json:"employee_id"`
	EmployeeNumber     string     `json:"employee_number,omitempty"`
	FullName           string     `json:"full_name,omitempty"`
	OperatorID         *string    `json:"operator_id"`
	OperatorNumber     *string    `json:"operator_number"`
	EventType          string     `json:"event_type"`
	TimestampClient    *time.Time `json:"timestamp_client"`
	TimestampServer    time.Time  `json:"timestamp_server"`
	Lat                *float64   `json:"lat"`
	Lon                *float64   `json:"lon"`
	AccuracyM          *float64   `json:"accuracy_m,omitempty"`
	LocationAgeSec     *int       `json:"location_age_sec,omitempty"`
	DeviceOS           *string    `json:"device_os,omitempty"`
	DeviceOSVersion    *string    `json:"device_os_version,omitempty"`
	DeviceModel        *string    `json:"device_model,omitempty"`
	AppVersion         *string    `json:"app_version,omitempty"`
	MockLocation       bool       `json:"mock_location"`
	OutsideOffice      bool       `json:"outside_office"`
	Overtime           bool       `json:"overtime"`
	AutoClosedPrevious bool       `json:"auto_closed_previous"`
	QRSource           *string    `json:"qr_source"`
	QRTokenID          *string    `json:"qr_token_id,omitempty"`
	QRDataMismatch     bool       `json:"qr_data_mismatch"`
	ClientType         *string    `json:"client_type"`
	ShiftID            *string    `json:"shift_id"`
	InvalidReason      *string    `json:"invalid_reason,omitempty"`
}

type MarkInput struct {
	EventType       string
	ClientType      string
	TimestampServer time.Time
	TimestampClient *time.Time
	Lat             *float64
	Lon             *float64
	AccuracyM       *float64
	LocationAgeSec  *int
	DeviceOS        *string
	DeviceOSVersion *string
	DeviceModel     *string
	AppVersion      *string
	MockLocation    bool
	QRTokenID       *string
	QRDataMismatch  bool
	OutsideOffice   bool
	Overtime        bool
	ScheduleID      *string
}

// AutoCloseInput описывает автоматическое закрытие предыдущей смены
// при получении новой отметки прихода.
type AutoCloseInput struct {
	At       time.Time
	Overtime bool
}

type AutoClosure struct {
	ShiftID  string
	MarkID   string
	ClosedAt time.Time
}

type MarkFilter struct {
	EmployeeID         string
	OperatorID         string
	From               *time.Time
	To                 *time.Time
	OutsideOffice      *bool
	MockLocation       *bool
	AutoClosedPrevious *bool
	Overtime           *bool
	QRSource           string
	ClientType         string
	QRDataMismatch     *bool
	Query              string
	DepartmentID       string
	PositionID         string
	Limit              int
	Offset             int
}

type Schedule struct {
	ID          string  `json:"id"`
	EmployeeID  string  `json:"employee_id"`
	PatternType string  `json:"pattern_type"`
	Workdays    []int   `json:"workdays"`
	StartTime   string  `json:"start_time"`
	EndTime     string  `json:"end_time"`
	ValidFrom   string  `json:"valid_from"`
	ValidTo     *string `json:"valid_to"`
	IsActive    bool    `json:"is_active"`
	SourceFile  *string `json:"source_file,omitempty"`

	EmployeeNumber   string `json:"employee_number,omitempty"`
	EmployeeFullName string `json:"employee_full_name,omitempty"`
}

type ScheduleTemplate struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	PatternType string `json:"pattern_type"`
	Workdays    []int  `json:"workdays"`
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
}

type ScheduleTemplateInput struct {
	Name        string
	PatternType string
	Workdays    []int
	StartTime   string
	EndTime     string
	CreatedBy   *string
}

type ScheduleInput struct {
	EmployeeID  string
	PatternType string
	Workdays    []int
	StartTime   string
	EndTime     string
	ValidFrom   string
	ValidTo     *string
	SourceFile  *string
	CreatedBy   *string
}

type Shift struct {
	ID         string     `json:"id"`
	EmployeeID string     `json:"employee_id"`
	ScheduleID *string    `json:"schedule_id"`
	StartedAt  time.Time  `json:"started_at"`
	EndedAt    *time.Time `json:"ended_at"`
	Status     string     `json:"status"`
	Overtime   bool       `json:"overtime"`
}

type Office struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
	RadiusM  int     `json:"radius_m"`
	IsActive bool    `json:"is_active"`
}

type OfficeInput struct {
	Name     string  `json:"name"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
	RadiusM  *int    `json:"radius_m"`
	IsActive *bool   `json:"is_active"`
}

type AuditEntry struct {
	ID          int64           `json:"id"`
	ActorUserID *string         `json:"actor_user_id"`
	Action      string          `json:"action"`
	EntityType  *string         `json:"entity_type"`
	EntityID    *string         `json:"entity_id"`
	Details     json.RawMessage `json:"details"`
	IP          *string         `json:"ip"`
	UserAgent   *string         `json:"user_agent,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

type AuditFilter struct {
	Action      string
	EntityType  string
	EntityID    string
	ActorUserID string
	IP          string
	From        *time.Time
	To          *time.Time
	Limit       int
	Offset      int
}

type Notification struct {
	ID           string          `json:"id"`
	Type         string          `json:"type"`
	Title        string          `json:"title"`
	Body         *string         `json:"body"`
	Payload      json.RawMessage `json:"payload"`
	TargetUserID *string         `json:"target_user_id"`
	IsRead       bool            `json:"is_read"`
	CreatedAt    time.Time       `json:"created_at"`
}

type Delegation struct {
	ID          string     `json:"id"`
	FromUserID  string     `json:"from_user_id"`
	ToUserID    string     `json:"to_user_id"`
	FromLogin   string     `json:"from_login,omitempty"`
	ToLogin     string     `json:"to_login,omitempty"`
	Status      string     `json:"status"`
	RequestedAt time.Time  `json:"requested_at"`
	ValidUntil  time.Time  `json:"valid_until"`
	DecidedBy   *string    `json:"decided_by"`
	DecidedAt   *time.Time `json:"decided_at"`
}

type OAuthClient struct {
	ID              string     `json:"id"`
	ClientID        string     `json:"client_id"`
	Name            *string    `json:"name"`
	IsActive        bool       `json:"is_active"`
	SecretRotatedAt *time.Time `json:"secret_rotated_at"`
	NextRotationAt  *time.Time `json:"next_rotation_at"`
}

type OAuthClientCreated struct {
	OAuthClient
	ClientSecret string `json:"client_secret"`
}

type OAuthAccess struct {
	ClientUUID string
	ClientID   string
	ExpiresAt  time.Time
}

type IpWhitelistEntry struct {
	ID            string  `json:"id"`
	OAuthClientID string  `json:"oauth_client_id"`
	CIDR          string  `json:"cidr"`
	Description   *string `json:"description"`
}

type ExportMark struct {
	MarkID           string     `json:"mark_id"`
	EmployeeNumber   string     `json:"employee_number"`
	EmployeeFullName string     `json:"employee_full_name"`
	EventType        string     `json:"event_type"`
	TimestampServer  time.Time  `json:"timestamp_server"`
	TimestampClient  *time.Time `json:"timestamp_client"`
	OperatorNumber   *string    `json:"operator_number"`
	QRSource         *string    `json:"qr_source"`
	ClientType       *string    `json:"client_type"`
	IsTest           bool       `json:"is_test"`
}
