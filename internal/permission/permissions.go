package permission

import "github.com/itzmanish/pratibimb-go/internal/role"

type Permission string
type Access string

const (
	CHANGE_ROOM_LOCK Permission = "CHANGE_ROOM_LOCK"
	PROMOTE_PEER     Permission = "PROMOTE_PEER"
	SEND_CHAT        Permission = "SEND_CHAT"
	MODERATE_CHAT    Permission = "MODERATE_CHAT"
	SHARE_SCREEN     Permission = "SHARE_SCREEN"
	EXTRA_VIDEO      Permission = "EXTRA_VIDEO"
	SHARE_FILE       Permission = "SHARE_FILE"
	MODERATE_FILES   Permission = "MODERATE_FILES"
	MODERATE_ROOM    Permission = "MODERATE_ROOM"
)
const (
	BYPASS_ROOM_LOCK Access = "BYPASS_ROOM_LOCK"
	BYPASS_LOBBY     Access = "BYPASS_LOBBY"
)

var RoomAccess = map[Access][]role.Role{
	BYPASS_ROOM_LOCK: {role.ADMIN},
	BYPASS_LOBBY:     {role.NORMAL},
}

var RoomPermissions = map[Permission][]role.Role{
	CHANGE_ROOM_LOCK: {role.NORMAL},
	PROMOTE_PEER:     {role.NORMAL},
	SEND_CHAT:        {role.NORMAL},
	SHARE_SCREEN:     {role.NORMAL},
	SHARE_FILE:       {role.NORMAL},
	MODERATE_CHAT:    {role.MODERATOR},
	MODERATE_FILES:   {role.MODERATOR},
	MODERATE_ROOM:    {role.MODERATOR},
}

var AllowWhenRoleMissing = []Permission{
	CHANGE_ROOM_LOCK,
}
