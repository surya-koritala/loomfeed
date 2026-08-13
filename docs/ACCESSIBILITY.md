# Accessibility

Loomfeed's modal baseline follows the
[WAI-ARIA Authoring Practices dialog pattern](https://www.w3.org/WAI/ARIA/apg/patterns/dialog-modal/):
an active modal has an accessible name, keeps focus inside, closes with
Escape, makes the page behind it inert, and restores focus when it closes.

`web/src/components/Dialog.tsx` owns that shared behavior. New modal surfaces
should use it rather than recreating an overlay. Its keyboard, focus,
background-interaction, and scroll-lock behavior is covered by
`Dialog.test.tsx`; `AccessibleModals.test.tsx` runs axe against the current
consumers and verifies their title and description associations.

## Overlay inventory

| Surface | Current status | Follow-up |
| --- | --- | --- |
| Post receipt (`PostReceipt.tsx`) | Uses the shared dialog primitive | Keep consumer-level axe coverage |
| Revision history (`RevisionModal.tsx`) | Uses the shared dialog primitive | Keep consumer-level axe coverage |
| Profile report (`views/Profile.tsx`) | Has partial dialog semantics but no shared focus/background behavior | Migrate to `Dialog` |
| Account deletion (`PrivacyDataSection.tsx`) | Visual modal with autofocus only | Migrate to `Dialog`; preserve destructive-action confirmation |
| Keyboard shortcut help (`KeyboardShortcuts.tsx`) | Has Escape handling but no dialog semantics or focus management | Migrate to `Dialog` without changing global shortcuts |
| Onboarding tour (`OnboardingTour.tsx`) | Has dialog semantics and Escape handling but incomplete focus/background behavior | Adapt to `Dialog` while preserving step navigation |
| Mobile navigation drawer (`LFMobileDrawer.tsx`) | Overlay navigation, not a modal dialog; locks scroll and makes the closed drawer inert | Review focus entry/restoration and containment as a navigation pattern |

When adding or changing an overlay, test it with keyboard-only navigation and
assistive technology in addition to the automated checks. Axe catches only a
subset of accessibility defects and does not replace manual interaction
testing.
