---
name: HitKeep
description: A calm, modern analytics console built for sustained focus and predictable operation.
colors:
  primary: "#10b981"
  primary-hover: "#059669"
  primary-active: "#047857"
  primary-deep: "#065f46"
  primary-deepest: "#064e3b"
  primary-soft: "#ecfdf5"
  primary-dark-mode: "#34d399"
  light-canvas: "#f8fafc"
  light-surface: "#ffffff"
  light-border: "#e2e8f0"
  light-text: "#334155"
  light-muted: "#64748b"
  dark-canvas: "#09090b"
  dark-surface: "#18181b"
  dark-border: "#3f3f46"
  dark-text: "#ffffff"
  dark-muted: "#a1a1aa"
  success: "#22c55e"
  info: "#3b82f6"
  warning: "#f59e0b"
  danger: "#ef4444"
typography:
  display:
    fontFamily: "Lexend Variable, ui-sans-serif, system-ui, sans-serif"
    fontSize: "2.25rem"
    fontWeight: 700
    lineHeight: 1.111
    letterSpacing: "-0.025em"
  headline:
    fontFamily: "Lexend Variable, ui-sans-serif, system-ui, sans-serif"
    fontSize: "1.5rem"
    fontWeight: 700
    lineHeight: 1.333
    letterSpacing: "-0.025em"
  title:
    fontFamily: "Lexend Variable, ui-sans-serif, system-ui, sans-serif"
    fontSize: "1rem"
    fontWeight: 700
    lineHeight: 1.25
    letterSpacing: "normal"
  body:
    fontFamily: "Lexend Variable, ui-sans-serif, system-ui, sans-serif"
    fontSize: "1rem"
    fontWeight: 400
    lineHeight: 1.5
    letterSpacing: "normal"
  label:
    fontFamily: "Lexend Variable, ui-sans-serif, system-ui, sans-serif"
    fontSize: "0.875rem"
    fontWeight: 600
    lineHeight: 1.429
    letterSpacing: "normal"
  small:
    fontFamily: "Lexend Variable, ui-sans-serif, system-ui, sans-serif"
    fontSize: "0.75rem"
    fontWeight: 600
    lineHeight: 1.333
    letterSpacing: "normal"
rounded:
  none: "0"
  xs: "2px"
  sm: "4px"
  md: "6px"
  lg: "8px"
  xl: "12px"
  pill: "9999px"
spacing:
  xxs: "4px"
  xs: "8px"
  sm: "12px"
  md: "16px"
  lg: "24px"
  xl: "32px"
components:
  button-primary:
    backgroundColor: "{colors.primary-active}"
    textColor: "{colors.light-surface}"
    typography: "{typography.label}"
    rounded: "{rounded.md}"
    padding: "0.5rem 0.75rem"
  button-primary-hover:
    backgroundColor: "{colors.primary-deep}"
    textColor: "{colors.light-surface}"
    typography: "{typography.label}"
    rounded: "{rounded.md}"
    padding: "0.5rem 0.75rem"
  button-secondary:
    backgroundColor: "{colors.light-surface}"
    textColor: "{colors.light-text}"
    typography: "{typography.label}"
    rounded: "{rounded.md}"
    padding: "0.5rem 0.75rem"
  input:
    backgroundColor: "{colors.light-surface}"
    textColor: "{colors.light-text}"
    typography: "{typography.body}"
    rounded: "{rounded.md}"
    padding: "0.5rem 0.75rem"
  card-flat:
    backgroundColor: "{colors.light-surface}"
    textColor: "{colors.light-text}"
    rounded: "{rounded.lg}"
    padding: "1rem"
  dialog-modal:
    backgroundColor: "{colors.light-surface}"
    textColor: "{colors.light-text}"
    rounded: "{rounded.xl}"
    padding: "1.25rem"
  table-compact:
    backgroundColor: "{colors.light-surface}"
    textColor: "{colors.light-text}"
    typography: "{typography.label}"
    rounded: "{rounded.xl}"
    padding: "0.5rem 0.75rem"
  chip-state:
    backgroundColor: "{colors.primary-soft}"
    textColor: "{colors.primary-active}"
    typography: "{typography.small}"
    rounded: "{rounded.pill}"
    padding: "0.35rem 0.55rem"
  navigation-item:
    backgroundColor: "{colors.light-surface}"
    textColor: "{colors.light-text}"
    typography: "{typography.label}"
    rounded: "{rounded.md}"
    padding: "0.5rem 0.75rem"
  navigation-item-active:
    backgroundColor: "{colors.primary-soft}"
    textColor: "{colors.primary-active}"
    typography: "{typography.label}"
    rounded: "{rounded.md}"
    padding: "0.5rem 0.75rem"
  feedback-error:
    backgroundColor: "#fef2f2"
    textColor: "#b91c1c"
    typography: "{typography.label}"
    rounded: "{rounded.lg}"
    padding: "0.75rem 0.875rem"
---

# Design System: HitKeep

## 1. Overview

**Creative North Star: "The Quiet Operations Console"**

HitKeep is a calm, trustworthy workspace for people who spend real time interpreting analytics and operating a product. The interface is deliberately easy on the eyes: visual groups are clear, placement is predictable, contrast is sufficient without becoming harsh, and motion never competes with the task. This sensory-conscious approach supports neurodivergent users and anyone affected by visual fatigue without reducing information density or making the product feel clinical.

Professional structure comes first. Modernity comes from Lexend, disciplined spacing, crisp semantic color, responsive controls, and small state transitions—not decorative effects. Every surface should make privacy, permissions, scope, loading, and one-time security boundaries understandable at the point of action.

The system rejects opaque ad-tech dashboards, dependency-heavy enterprise administration, and decorative SaaS surfaces. A screen succeeds when an experienced operator can predict where actions, status, filters, and feedback will appear before reading every label.

**Key Characteristics:**

- Calm, restrained surfaces with one emerald action signal.
- Consistent light and dark modes built from semantic OptimusUI tokens.
- Dense but orderly information with stable placement during loading and refresh.
- Shared controls and interaction harnesses before page-local markup.
- Explicit, localized feedback close to the action that caused it.
- Keyboard access, visible focus, reduced-motion support, and no reliance on color alone.

**The Quiet Surface Rule.** Remove competing emphasis, not useful information. One region leads; supporting regions remain legible and structurally stable.

**The Predictability Rule.** Equivalent tasks must use the same component, placement, loading behavior, and feedback pattern across the dashboard.

## 2. Colors

The palette combines an emerald action signal with slate daylight surfaces and zinc night surfaces. It is restrained by default: semantic state colors communicate meaning, while the primary accent marks action, selection, focus, and live status rather than decoration.

### Primary

- **Emerald Signal** (`primary`, `primary-hover`, `primary-active`): primary actions, current navigation, selected controls, focus, and live progress.
- **Emerald Quiet** (`primary-soft`): low-emphasis selected or highlighted backgrounds in light mode.
- **Emerald Night Signal** (`primary-dark-mode`): the higher-lightness primary used by Aura in dark mode.

### Neutral

- **Slate Daylight** (`light-canvas`, `light-surface`, `light-border`): the light-mode canvas, content surfaces, dividers, and control boundaries.
- **Slate Ink** (`light-text`, `light-muted`): primary and secondary light-mode text. Muted text remains readable and is never used to hide required information.
- **Zinc Night** (`dark-canvas`, `dark-surface`, `dark-border`): the dark-mode canvas, raised surface, and structural boundary vocabulary.
- **Night Ink** (`dark-text`, `dark-muted`): high-contrast content and supporting dark-mode text.

### Tertiary

- **Semantic State Family** (`success`, `info`, `warning`, `danger`): status, validation, notices, destructive actions, and chart meaning. Pair every state color with text, an icon, or another non-color cue.

### Named Rules

**The One Signal Rule.** Emerald is the only general-purpose accent and should occupy no more than roughly 10% of a screen. Additional hues require semantic or data-visualization meaning.

**The Semantic Theme Rule.** New UI must consume OptimusUI semantic variables or a project OptimusUI preset. Never hardcode separate light and dark colors when one semantic token can express both.

**The Comfortable Contrast Rule.** Body and placeholder text must meet WCAG AA. “Easy on the eyes” means controlled contrast and hierarchy, never faint copy or low-contrast controls.

## 3. Typography

**Display Font:** Lexend Variable with the system sans-serif stack.
**Body Font:** Lexend Variable with the system sans-serif stack.
**Label/Mono Font:** Lexend Variable for UI labels; the system monospace stack only for credentials, identifiers, code, and technical evidence.

**Character:** Lexend gives the product a friendly modern cadence while remaining practical at dashboard sizes. One family keeps the interface coherent and reduces typographic noise; weight, size, spacing, and placement carry hierarchy.

### Hierarchy

- **Display** (`typography.display`): rare overview or authentication headings; never ordinary page chrome.
- **Headline** (`typography.headline`): page titles and major analytical statements.
- **Title** (`typography.title`): cards, panels, dialogs, and grouped controls.
- **Body** (`typography.body`): instructions, values, table-supporting copy, and explanatory prose; long prose stays within 65–75 characters per line.
- **Label** (`typography.label`): buttons, controls, table headings, and form labels.
- **Small** (`typography.small`): metadata, compact status, and secondary table information; it must remain high enough in contrast to read comfortably.

### Named Rules

**The One-Family Rule.** Use Lexend for product UI. Do not introduce display fonts, a second sans-serif, or decorative type for individual features.

**The Fixed Product Scale Rule.** Dashboard typography uses the fixed scale in the frontmatter. Do not use fluid display sizing inside authenticated product surfaces.

**The Quiet Emphasis Rule.** Prefer weight, spacing, and position over uppercase tracking. Repeated tiny all-caps eyebrows are prohibited.

## 4. Elevation

HitKeep is flat by default and layered by structure. Borders and surface tones separate persistent regions; small shadows support form fields and low surfaces; pronounced shadows are reserved for overlays that genuinely sit above the application. Light and dark modes use the same elevation roles, with dark surfaces relying more on tonal separation.

### Shadow Vocabulary

- **Surface Low** (`0 1px 2px rgb(15 23 42 / 0.04)`; dark: `0 1px 2px rgb(0 0 0 / 0.28)`): shared flat cards where a border alone needs slight separation.
- **Field Rest** (`0 1px 2px 0 rgba(18, 18, 23, 0.05)`): Aura form controls at rest.
- **Overlay Select** (`0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -2px rgba(0, 0, 0, 0.1)`): menus, popovers, and select panels.
- **Overlay Modal** (`0 20px 25px -5px rgba(0, 0, 0, 0.1), 0 8px 10px -6px rgba(0, 0, 0, 0.1)`): dialogs and modal overlays only.

### Named Rules

**The Flat-by-Default Rule.** Persistent content surfaces use a border or a shallow shadow, not both as decoration. Large shadows belong only to overlays.

**The Structural Depth Rule.** If removing a shadow does not make the stacking relationship less clear, remove it.

## 5. Components

**Component philosophy: shared, native, and predictable.** OptimusUI is the control layer; Tailwind and component CSS arrange those controls. New features must first reuse a OptimusUI component, then an existing HitKeep shared component, then extract a shared component when a pattern repeats. Page-local control chrome is the last resort.

**The OptimusUI-First Rule.** Control sizing, radii, surface colors, borders, focus rings, state colors, dialog spacing, table density, and both color schemes belong in a project preset derived from Aura whenever OptimusUI exposes the token. Do not duplicate this styling across pages.

**The Shared-Before-New Rule.** Search the shared component set before authoring markup. If the same interaction or chrome appears twice, extract it instead of copying classes or templates.

### Buttons

- **Shape:** gently curved Aura controls (`rounded.md`) with compact, stable dimensions.
- **Primary:** use the contrast-safe deep emerald mapping in light mode with a white label; use the brighter dark-mode emerald with dark zinc ink. Put this scheme-specific mapping in the OptimusUI preset. Preserve width during loading and use an optional leading OpenNG icon only when it improves recognition.
- **Secondary:** use OptimusUI outlined, text, or secondary severity variants; do not create feature-specific button skins.
- **Hover / Focus / Active:** use Aura semantic states. Focus remains clearly visible with the primary ring and offset.
- **Destructive:** use OptimusUI danger severity and confirmation helpers. Color is reinforced by explicit copy and context.

### Chips

- **Style:** reserve pills for compact roles, states, filters, and tags. Use OptimusUI Tag/Chip severities or shared role chips.
- **State:** selected and unselected treatments use semantic backgrounds and readable labels; never make an inactive chip look disabled unless it is disabled.

### Cards / Containers

- **Settings and forms:** use the shared `SettingsCard` with its header, body, and footer slots.
- **Bounded pages:** use the shared `PageFrame` for breadcrumb, header, and the 72rem content boundary.
- **General grouped content:** use OptimusUI Card with the shared flat-card treatment. Move reusable card chrome into the OptimusUI preset rather than adding more global overrides.
- **Analytics:** use `KpiCard` and `MetricCardGroup` for their intended analytical patterns.
- **Corner Style:** use `rounded.lg` for ordinary cards and `rounded.xl` only where the OptimusUI component already calls for it.
- **Structure:** cards group related content; they do not wrap every section, and cards are never nested for decoration.

### Inputs / Fields

- **Style:** use OptimusUI form components and their semantic field tokens. Labels remain visible; placeholder text is supporting copy, not a label substitute.
- **Search:** use OptimusUI IconField, InputIcon, and InputText with the established shared search sizing.
- **Focus:** use the Aura primary border/focus treatment; do not create local glows.
- **Error / Disabled:** use OptimusUI invalid and disabled states. Keep error text adjacent to the field, localized, and connected semantically.

### Navigation

- **Style:** the existing sidebar uses compact rows, a quiet hover surface, emerald current-state text, and a tinted current-state background.
- **Hierarchy:** section labels organize navigation without adding decorative kickers elsewhere.
- **Responsive behavior:** preserve the desktop sidebar and shared mobile drawer/header patterns. Do not invent page-specific navigation.

### Inline Feedback and Page States

- **Routine feedback:** use a localized OptimusUI Message adjacent to the affected surface. Form submission errors remain inside the still-open dialog; successful completion appears near the updated list or object.
- **Rich credential feedback:** use `OneTimeCredential` and `CopyControl` for generated secrets, copy status, and polite live announcements.
- **Empty, forbidden, and unavailable pages:** use `PageState` with an icon, title, useful explanation, and an action when one exists. Empty states teach the next step; they never say only “nothing here.”
- **Persistent notices:** reuse specialized shared notices when their lifecycle and dismissal rules match.

### Searchable, Sortable, Paginated Tables

- **Ordinary CRUD tables:** compose OptimusUI Table with the existing global CRUD table helpers. The standard includes compact density, responsive horizontal scrolling, row hover, a localized search field, visible sort affordances, and a final shared row-action menu.
- **Pagination:** default to 10 rows with 10/25/50 page-size options for client-paginated CRUD tables unless the domain requires a different scale.
- **Sorting:** every sortable heading uses both OptimusUI's sortable-column directive and matching sort icon; declare an intentional initial sort.
- **Search:** declare the actual global-filter fields and use the established search control; never display a search box that filters only some columns without explaining the scope.
- **Actions:** use `TableRowActions`, including its danger state and row-scoped loading. Do not line a row with competing icon buttons.
- **Server pagination:** use `AuditTableComponent` for audit-log behavior. Keep its query, debounce, reset-to-first-page, expansion, export, and localized range-summary contract intact.
- **Loading:** preserve existing rows during background refresh where possible. Use skeletons for first load and stable loading states for ongoing work; do not flash the table blank.

### Dialogs and CRUD Flows

- **General dialogs:** use `DialogShell`. It standardizes the OptimusUI modal, body portal, 42rem default width, 96vw responsive width, action footer, busy state, and dismissal lock.
- **Create and edit forms:** use `CrudDialog`, which wraps `DialogShell` and standardizes cancel, submit, saving, and closure behavior.
- **Destructive confirmation:** use OptimusUI ConfirmDialog with `ConfirmationService` and the shared cancel/primary/danger/warn button helpers.
- **CRUD lifecycle:** reset form and dialog errors before opening; validate and mark touched before sending; block duplicate submits; keep failure feedback in the open dialog; on success close/reset and update the list before announcing success; show new secrets through `OneTimeCredential`; reset scoped dialog state when the active site or team changes.
- **Mobile:** footer actions become full-width when space is constrained. Menus and dialogs must portal outside clipped scrolling containers.

### Theme, Motion, and Charts

- **Theme:** Aura is configured with `.p-dark`; `PreferencesService` owns the persisted theme signal. Every changed surface must be checked in both modes.
- **Custom CSS:** use `--p-content-background`, `--p-content-border-color`, `--p-text-color`, `--p-text-muted-color`, `--p-primary-color`, and surface ramps. Paired light/dark selectors are allowed only for real product-specific differences that semantic tokens cannot represent.
- **Motion:** standard control transitions use Aura's 200ms duration. Motion communicates state and must honor reduced-motion preferences; never add orchestrated product-page entrances.
- **Charts:** use the shared HitKeep chart theme and option builder so text, grid, tooltip, accessibility, and light/dark behavior stay consistent.

## 6. Do's and Don'ts

### Do:

- **Do** make privacy, permissions, scope, destructive consequences, and one-time credentials visible at the point of action.
- **Do** keep one clear visual lead per region while preserving useful information and stable placement.
- **Do** use OptimusUI's components, severities, states, templates, accessibility, and theme tokens before custom control markup or CSS.
- **Do** reuse `SettingsCard`, `PageFrame`, `DialogShell`, `CrudDialog`, `TableRowActions`, `PageState`, `CopyControl`, `OneTimeCredential`, `KpiCard`, `MetricCardGroup`, and `AuditTableComponent` when their contracts fit.
- **Do** make ordinary CRUD tables searchable, intentionally sortable, paginated, responsive, and keyboard accessible using the established OptimusUI composition.
- **Do** keep errors near the failed action, keep dialog errors inside the dialog, and announce successful state changes near the updated surface.
- **Do** test every changed surface in light mode, dark mode, keyboard navigation, narrow layout, and reduced motion.
- **Do** maintain WCAG AA contrast and reinforce semantic color with text, iconography, or structure.
- **Do** put every user-facing string in Transloco and preserve behavior across all seven supported locales.

### Don't:

- **Don't** create opaque ad-tech dashboards that conceal data collection, privacy, permission, or security boundaries.
- **Don't** create dependency-heavy enterprise administration interfaces that make routine operations feel complex.
- **Don't** create decorative SaaS surfaces that trade information hierarchy and legibility for visual novelty.
- **Don't** duplicate OptimusUI control chrome in page CSS; extend the shared Aura preset or use semantic variables.
- **Don't** copy a repeated interaction or card treatment into another feature; use or extract a shared component.
- **Don't** interpret “easy on the eyes” as faint text, hidden labels, ambiguous icons, excessive empty space, or missing information.
- **Don't** use decorative gradients, gradient text, default glassmorphism, diagonal stripes, decorative grid backgrounds, or large ambient glows.
- **Don't** use colored side-stripe accents, nested cards, or a border plus a wide decorative shadow.
- **Don't** over-round cards, dialogs, or inputs; ordinary surfaces stop at `rounded.xl`, while full pills are reserved for tags and compact controls.
- **Don't** rely on color alone, introduce flashing or decorative motion, or animate layout properties during ordinary state changes.
- **Don't** replace standard OptimusUI affordances with novel controls that make an experienced operator pause.
