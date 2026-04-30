package store

// Role is the user permission tier. Stored as a TEXT column with a CHECK
// constraint; ordered by AtLeast for permission gates. Lives in the store
// package (rather than auth) because the type is the column type, and
// auth already imports store — the reverse would create a cycle.
type Role string

const (
	RoleGuest Role = "guest" // read-only spectator; no password required
	RoleUser  Role = "user"  // registered, default after register
	RoleMod   Role = "mod"   // can delete anyone's article/push
	RoleAdmin Role = "admin" // can promote/demote others
)

var roleRank = map[Role]int{
	RoleGuest: 0,
	RoleUser:  1,
	RoleMod:   2,
	RoleAdmin: 3,
}

// AtLeast returns true if r is at or above min in the rank table.
// Unknown roles always return false (fail-closed).
func (r Role) AtLeast(min Role) bool {
	rr, rok := roleRank[r]
	mr, mok := roleRank[min]
	if !rok || !mok {
		return false
	}
	return rr >= mr
}

// Valid reports whether r is one of the four known role values.
func (r Role) Valid() bool {
	_, ok := roleRank[r]
	return ok
}
