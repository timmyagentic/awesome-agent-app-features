// Package feedback builds provider-neutral, redacted feedback reports and
// records explicit user approval.
//
// The package deliberately stops at the product boundary. Host applications
// own rendering, interaction, permission, and localization. Transport and
// downstream issue formatting live in adapter packages. Report is a
// non-serializable preview; Approved is the only serializable report type, so
// an adapter has to make approval explicit. Version 1 emits Feedback wire v1.
package feedback
