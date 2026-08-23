// Package feedback builds redacted, user-approved feedback reports and sends
// them to an author-operated relay.
//
// The package deliberately stops at the product boundary. Host applications
// own the button, card, natural-language intent, permission, and localization
// layers. The only submission API accepts an Approved report, so an adapter
// has to make the approval step explicit in code.
package feedback
