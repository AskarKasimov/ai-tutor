package user

type Role string

const (
	Student Role = "student"
	Admin   Role = "admin"
)

type User struct {
	ID          string
	Email       string
	DisplayName *string
	Role        Role
	CreatedAt   int64
}
