package apiroutes

const (
	APIV1Prefix = "/api/v1"

	AdminGroupPath = "/admin"
	UserGroupPath  = "/user"

	ProfilePath      = "/profile"
	PasswordPath     = "/password"
	AccountLinksPath = "/account-links"
	CurrentPath      = "/current"

	FullProfilePath      = APIV1Prefix + ProfilePath
	FullPasswordPath     = APIV1Prefix + PasswordPath
	FullAccountLinksPath = APIV1Prefix + AccountLinksPath
	FullAdminCurrentPath = APIV1Prefix + AdminGroupPath + CurrentPath
	FullUserProfilePath  = APIV1Prefix + UserGroupPath + ProfilePath
	FullUserPasswordPath = APIV1Prefix + UserGroupPath + PasswordPath
)

var passwordResetClosedLoopPaths = [...]string{
	FullProfilePath,
	FullPasswordPath,
	FullAccountLinksPath,
	FullAdminCurrentPath,
	FullUserProfilePath,
	FullUserPasswordPath,
}

func PasswordResetClosedLoopPaths() []string {
	paths := make([]string, 0, len(passwordResetClosedLoopPaths))
	paths = append(paths, passwordResetClosedLoopPaths[:]...)
	return paths
}
