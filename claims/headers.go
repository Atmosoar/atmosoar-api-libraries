// Package claims is the single source of truth for the Atmosoar gateway-trust
// contract: the X-Atmosoar-* header names the gateway stamps after validating a
// JWT, and the Gin middleware backend services use to parse those headers into a
// typed identity.
//
// Trust model: every backend (mma, radar, observation, metar, elevation, fdi) is
// deployed behind atmosoar-gateway as an internal-only ClusterIP service. A
// NetworkPolicy restricts ingress to the gateway pod, so only the gateway can set
// the X-Atmosoar-* headers. Before forwarding, the gateway strips any
// client-supplied copies (see AllTrustHeaders) and sets its own from the
// validated JWT claims. Backends read them trusting the network boundary.
//
// This package replaces the per-service "LOCAL MIRROR" copies that previously
// duplicated these constants (multi-model-api/middleware/claims, radar, etc.) and
// the gateway's internal/proxy/headers.go. Renaming any header below, or changing
// its meaning, is a breaking contract change requiring an identity-version bump
// (see CurrentIdentityVersion). Adding a NEW optional header (as Orgs/Teams and
// Admin were) is additive — existing readers ignore extras — so the version stays
// "1".
package claims

// Gateway-trust header names forwarded to upstream services after JWT validation.
const (
	HeaderUserEmail = "X-Atmosoar-User-Email"
	HeaderUserSub   = "X-Atmosoar-User-Sub"
	HeaderUserTier  = "X-Atmosoar-User-Tier"
	HeaderUserRoles = "X-Atmosoar-User-Roles"
	// HeaderUserOrgs / HeaderUserTeams carry the caller's Keycloak org/team group
	// paths (comma-joined, may be empty), resolved from the "groups" JWT claim and
	// consumed by FOB to scope tenancy without a DB membership table.
	HeaderUserOrgs  = "X-Atmosoar-User-Orgs"
	HeaderUserTeams = "X-Atmosoar-User-Teams"
	// HeaderIdentityVersion pins the contract version; backends reject mismatches.
	HeaderIdentityVersion = "X-Atmosoar-Identity-Version"
	// HeaderAdmin is set to "1" by the gateway ONLY for callers holding the
	// api:admin realm role, and ONLY on admin-policy proxied routes. It is the
	// backend authorization signal for /admin/<svc>/* endpoints. Because it is in
	// AllTrustHeaders, the gateway strips any client-supplied copy at the trust
	// boundary before (conditionally) stamping its own — so a client cannot forge
	// admin access. It is deliberately NOT in PropagatedTrustHeaders: admin
	// privilege is not propagated east-west between backends.
	HeaderAdmin = "X-Atmosoar-Admin"
)

// AdminHeaderValue is the only value the gateway writes into HeaderAdmin and the
// only value RequireAdmin accepts.
const AdminHeaderValue = "1"

// CurrentIdentityVersion is the value the gateway writes into
// HeaderIdentityVersion on every forwarded request, and the only value backends
// accept. Bumping it requires a coordinated gateway+backend roll.
const CurrentIdentityVersion = "1"

// AllTrustHeaders returns every header name the gateway owns on the trust
// boundary, including HeaderAdmin. The gateway strips every one of these from
// inbound client requests before setting its own — this is the list
// StripTrustHeaders iterates. Adding HeaderAdmin here is what closes the
// admin-header spoofing hole.
func AllTrustHeaders() []string {
	return []string{
		HeaderUserEmail,
		HeaderUserSub,
		HeaderUserTier,
		HeaderUserRoles,
		HeaderUserOrgs,
		HeaderUserTeams,
		HeaderIdentityVersion,
		HeaderAdmin,
	}
}

// PropagatedTrustHeaders returns the subset of trust headers a backend captures
// on the way in and replays verbatim on east-west calls to peer services. It
// excludes Orgs/Teams (matching the pre-existing per-service behavior) and
// HeaderAdmin (admin privilege must originate from the gateway, never be
// forwarded between backends).
func PropagatedTrustHeaders() []string {
	return []string{
		HeaderUserEmail,
		HeaderUserSub,
		HeaderUserTier,
		HeaderUserRoles,
		HeaderIdentityVersion,
	}
}
