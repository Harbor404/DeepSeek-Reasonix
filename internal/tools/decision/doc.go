// Package decision implements a stateless comparison tool over caller-supplied
// numeric evidence. It preserves explicit constraints and preference directions,
// marks unusable evidence, and reports a conditional Pareto frontier using exact
// decimal arithmetic. Source identities provide traceability, not verification.
// The shared capability catalog exposes it without another model invocation.
// Runtime answer checks bind to bounded immutable request snapshots. Production
// snapshots persist per workspace; evidence updates create linked new versions.
// Stateless constructors support experiments; neither path verifies prose.
package decision
