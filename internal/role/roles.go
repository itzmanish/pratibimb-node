package role

type Role string

const (
	ADMIN         Role = "ADMIN"
	MODERATOR     Role = "MODERATOR"
	PRESENTER     Role = "PRESENTER"
	AUTHENTICATED Role = "AUTHENTICATED"
	NORMAL        Role = "NORMAL"
)
