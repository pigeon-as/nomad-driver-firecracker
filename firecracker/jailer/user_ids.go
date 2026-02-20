package jailer

import (
	"os/user"
	"strconv"
)

func ResolveUserIDs(userStr string) (*int, *int, error) {
	var u *user.User
	var err error
	if _, err = strconv.Atoi(userStr); err == nil {
		u, err = user.LookupId(userStr)
	} else {
		u, err = user.Lookup(userStr)
	}
	if err != nil {
		return nil, nil, err
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return nil, nil, err
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return &uid, nil, err
	}
	return &uid, &gid, nil
}
