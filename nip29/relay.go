package nip29

import (
	"fmt"

	"github.com/nbd-wtf/go-nostr"
)

var (
	// used for the default role, the actual relay, hidden otherwise
	MasterRole *Role = &Role{
		Name: "master",
		Permissions: map[Permission]struct{}{
			PermAddUser:          {},
			PermEditMetadata:     {},
			PermDeleteEvent:      {},
			PermRemoveUser:       {},
			PermAddPermission:    {},
			PermRemovePermission: {},
			PermEditGroupStatus:  {},
		},
	}

	// used for normal members without admin powers, not displayed
	EmptyRole *Role = nil

	PermissionsMap = map[Permission]struct{}{
		PermAddUser:          {},
		PermEditMetadata:     {},
		PermDeleteEvent:      {},
		PermRemoveUser:       {},
		PermAddPermission:    {},
		PermRemovePermission: {},
		PermEditGroupStatus:  {},
	}
)

type Action interface {
	Apply(group *Group)
	PermissionName() Permission
}

func GetModerationAction(evt *nostr.Event) (Action, error) {
	factory, ok := moderationActionFactories[evt.Kind]
	if !ok {
		return nil, fmt.Errorf("event kind %d is not a supported moderation action", evt.Kind)
	}
	return factory(evt)
}

var moderationActionFactories = map[int]func(*nostr.Event) (Action, error){
	nostr.KindSimpleGroupAddUser: func(evt *nostr.Event) (Action, error) {
		targets := make([]string, 0, len(evt.Tags))
		for _, tag := range evt.Tags.GetAll([]string{"p", ""}) {
			if !nostr.IsValidPublicKeyHex(tag[1]) {
				return nil, fmt.Errorf("")
			}
			targets = append(targets, tag[1])
		}
		if len(targets) > 0 {
			return &AddUser{Targets: targets}, nil
		}
		return nil, fmt.Errorf("missing 'p' tags")
	},
	nostr.KindSimpleGroupRemoveUser: func(evt *nostr.Event) (Action, error) {
		targets := make([]string, 0, len(evt.Tags))
		for _, tag := range evt.Tags.GetAll([]string{"p", ""}) {
			if !nostr.IsValidPublicKeyHex(tag[1]) {
				return nil, fmt.Errorf("invalid public key hex")
			}
			targets = append(targets, tag[1])
		}
		if len(targets) > 0 {
			return &RemoveUser{Targets: targets}, nil
		}
		return nil, fmt.Errorf("missing 'p' tags")
	},
	nostr.KindSimpleGroupEditMetadata: func(evt *nostr.Event) (Action, error) {
		ok := false
		edit := EditMetadata{When: evt.CreatedAt}
		if t := evt.Tags.GetFirst([]string{"name", ""}); t != nil {
			edit.NameValue = (*t)[1]
			ok = true
		}
		if t := evt.Tags.GetFirst([]string{"picture", ""}); t != nil {
			edit.PictureValue = (*t)[1]
			ok = true
		}
		if t := evt.Tags.GetFirst([]string{"about", ""}); t != nil {
			edit.AboutValue = (*t)[1]
			ok = true
		}
		if ok {
			return &edit, nil
		}
		return nil, fmt.Errorf("missing metadata tags")
	},
	nostr.KindSimpleGroupAddPermission: func(evt *nostr.Event) (Action, error) {
		nTags := len(evt.Tags)

		permissions := make([]Permission, 0, nTags-1)
		for _, tag := range evt.Tags.GetAll([]string{"permission", ""}) {
			perm := Permission(tag[1])
			if _, ok := PermissionsMap[perm]; !ok {
				return nil, fmt.Errorf("unknown permission '%s'", tag[1])
			}
			permissions = append(permissions, perm)
		}

		targets := make([]string, 0, nTags-1)
		for _, tag := range evt.Tags.GetAll([]string{"p", ""}) {
			if !nostr.IsValidPublicKeyHex(tag[1]) {
				return nil, fmt.Errorf("invalid public key hex")
			}
			targets = append(targets, tag[1])
		}

		if len(permissions) > 0 && len(targets) > 0 {
			return &AddPermission{Targets: targets, Permissions: permissions}, nil
		}

		return nil, fmt.Errorf("")
	},
	nostr.KindSimpleGroupRemovePermission: func(evt *nostr.Event) (Action, error) {
		nTags := len(evt.Tags)

		permissions := make([]Permission, 0, nTags-1)
		for _, tag := range evt.Tags.GetAll([]string{"permission", ""}) {
			perm := Permission(tag[1])
			if _, ok := PermissionsMap[perm]; !ok {
				return nil, fmt.Errorf("unknown permission '%s'", tag[1])
			}
			permissions = append(permissions, perm)
		}

		targets := make([]string, 0, nTags-1)
		for _, tag := range evt.Tags.GetAll([]string{"p", ""}) {
			if !nostr.IsValidPublicKeyHex(tag[1]) {
				return nil, fmt.Errorf("invalid public key hex")
			}
			targets = append(targets, tag[1])
		}

		if len(permissions) > 0 && len(targets) > 0 {
			return &RemovePermission{Targets: targets, Permissions: permissions}, nil
		}

		return nil, fmt.Errorf("")
	},
	nostr.KindSimpleGroupDeleteEvent: func(evt *nostr.Event) (Action, error) {
		tags := evt.Tags.GetAll([]string{"e", ""})
		if len(tags) == 0 {
			return nil, fmt.Errorf("missing 'e' tag")
		}

		targets := make([]string, len(tags))
		for i, tag := range tags {
			if nostr.IsValidPublicKeyHex(tag[1]) {
				targets[i] = tag[1]
			} else {
				return nil, fmt.Errorf("invalid event id hex")
			}
		}

		return &DeleteEvent{Targets: targets}, nil
	},
	nostr.KindSimpleGroupEditGroupStatus: func(evt *nostr.Event) (Action, error) {
		egs := EditGroupStatus{When: evt.CreatedAt}

		egs.Public = evt.Tags.GetFirst([]string{"public"}) != nil
		egs.Private = evt.Tags.GetFirst([]string{"private"}) != nil
		egs.Open = evt.Tags.GetFirst([]string{"open"}) != nil
		egs.Closed = evt.Tags.GetFirst([]string{"closed"}) != nil

		// disallow contradictions
		if egs.Public && egs.Private {
			return nil, fmt.Errorf("contradiction")
		}
		if egs.Open && egs.Closed {
			return nil, fmt.Errorf("contradiction")
		}

		// TODO remove this once we start supporting private groups
		if egs.Private {
			return nil, fmt.Errorf("private groups not yet supported")
		}

		return egs, nil
	},
}

type DeleteEvent struct {
	Targets []string
}

func (DeleteEvent) PermissionName() Permission { return PermDeleteEvent }
func (a DeleteEvent) Apply(group *Group)       {}

type AddUser struct {
	Targets []string
}

func (AddUser) PermissionName() Permission { return PermAddUser }
func (a AddUser) Apply(group *Group) {
	for _, target := range a.Targets {
		group.Members[target] = EmptyRole
	}
}

type RemoveUser struct {
	Targets []string
}

func (RemoveUser) PermissionName() Permission { return PermRemoveUser }
func (a RemoveUser) Apply(group *Group) {
	for _, target := range a.Targets {
		delete(group.Members, target)
	}
}

type EditMetadata struct {
	NameValue    string
	PictureValue string
	AboutValue   string
	When         nostr.Timestamp
}

func (EditMetadata) PermissionName() Permission { return PermEditMetadata }
func (a EditMetadata) Apply(group *Group) {
	group.Name = a.NameValue
	group.Picture = a.PictureValue
	group.About = a.AboutValue
	group.LastMetadataUpdate = a.When
}

type AddPermission struct {
	Targets     []string
	Permissions []Permission
}

func (AddPermission) PermissionName() Permission { return PermAddPermission }
func (a AddPermission) Apply(group *Group) {
	for _, target := range a.Targets {
		role, ok := group.Members[target]

		// if it's a normal user, create a new permissions object thing for this user
		// instead of modifying the global EmptyRole
		if !ok || role == EmptyRole {
			role = &Role{Permissions: make(map[Permission]struct{})}
			group.Members[target] = role
		}

		// add all permissions listed
		for _, perm := range a.Permissions {
			role.Permissions[perm] = struct{}{}
		}
	}
}

type RemovePermission struct {
	Targets     []string
	Permissions []Permission
}

func (RemovePermission) PermissionName() Permission { return PermRemovePermission }
func (a RemovePermission) Apply(group *Group) {
	for _, target := range a.Targets {
		role, ok := group.Members[target]
		if !ok || role == EmptyRole {
			continue
		}

		// remove all permissions listed
		for _, perm := range a.Permissions {
			delete(role.Permissions, perm)
		}

		// if no more permissions are available, change this guy to be a normal user
		if role.Name == "" && len(role.Permissions) == 0 {
			group.Members[target] = EmptyRole
		}
	}
}

type EditGroupStatus struct {
	Public  bool
	Private bool
	Open    bool
	Closed  bool
	When    nostr.Timestamp
}

func (EditGroupStatus) PermissionName() Permission { return PermEditGroupStatus }
func (a EditGroupStatus) Apply(group *Group) {
	if a.Public {
		group.Private = false
	} else if a.Private {
		group.Private = true
	}

	if a.Open {
		group.Closed = false
	} else if a.Closed {
		group.Closed = true
	}

	group.LastMetadataUpdate = a.When
}
