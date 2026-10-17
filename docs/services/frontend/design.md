# SCYG Blog Desktop Design System

Source paths in this document are relative to the repository's [`frontend/`](../../../frontend/) directory. Service documentation is indexed in [README.md](README.md).

## 0. Research and scope

- Baseline: the existing semantic primitive names remain stable across two complete themes. `night` preserves the approved deep-night surface, while `sky` expresses the same hierarchy through clear blue atmosphere and white cloud surfaces.
- Theme ownership: `src/assets/themes/shared.css` owns only theme-independent typography, spacing, geometry, motion, and layout tokens. `night.css` and `sky.css` own identical color, shadow, browser chrome, and Hero token sets. Raw colors are allowed only in `src/assets/themes/*.css` and this document; `main.css`, Tailwind configuration, TypeScript, and Vue components consume semantic variables only.
- Visual source: `night` uses the project-owned starry welcome image; `sky` uses layered CSS gradients and requires no additional asset. The project-owned portrait supplies author identity. No external logo, copy, or proprietary visual asset is part of this system.
- Layout grammar: one homepage with mutually exclusive Hero and reading scenes. The Hero is exactly `100dvh`; explicit entry or downward wheel intent swaps it for the editorial canvas. Navigation is transparent on the Hero and solid on content.
- Support boundary: desktop only. The validated target is Microsoft Edge at 1440 x 900. Widths below `1024px` are outside this phase and must not create new mobile or tablet requirements.

## 1. Identity and principles

The public blog supports two coherent atmospheric readings. `night` moves from a photographic star field into a near-black navy reading canvas; `sky` moves from a layered clear-sky and soft-cloud Hero into an airy pale-blue canvas with white reading surfaces. Both preserve the same editorial hierarchy, exclusive scene entry, semantic accent usage, component geometry, and interaction behavior.

Three rules govern the surface:

1. Content remains the focal point. The selected atmosphere creates context while theme-specific surfaces and contrast-safe text support long CJK reading.
2. Depth is restrained and directional. Cards lift only on interaction; the fixed navigation changes from transparent Hero overlay to an opaque canvas-aligned reading state.
3. Motion communicates state. Hover, focus, navigation settling, editor save states, and the directional ScrollCue may transition; there is no continuous canvas or purposeless decorative looping animation.

## 2. Semantic color system

Every CSS and Tailwind semantic color maps to this table. Raw visual colors outside this contract are not allowed.

| Role | CSS token | Night | Sky | Use |
|---|---|---:|---:|---|
| Browser theme | `--color-browser-theme` | `#070b18` | `#eef1fa` | Browser chrome synchronized at runtime |
| Transparent | `--color-transparent` | `transparent` | `transparent` | Theme-declared transparent surfaces |
| Canvas | `--color-canvas` | `#070b18` | `#eef1fa` | Application and reading background |
| Surface | `--color-surface` | `#0d1628` | `#fffcfe` | Cards, panels, controls, and content-state Header |
| Surface muted | `--color-surface-muted` | `#131f34` | `#f4f2fa` | Metadata bands, footer, code blocks, and quiet controls |
| Surface hover | `--color-surface-hover` | `#1a2942` | `#e8eaf7` | Interactive surface hover |
| Text primary | `--color-text-primary` | `#f3f6fc` | `#293956` | Titles and body copy |
| Text secondary | `--color-text-secondary` | `#b6c1d4` | `#62708a` | Summaries and metadata |
| Text tertiary | `--color-text-tertiary` | `#8492aa` | `#8994aa` | Placeholders and timestamps |
| Border | `--color-border` | `#2b3a55` | `#d4d8ea` | Controls and strong divisions |
| Border subtle | `--color-border-subtle` | `#1d2a40` | `#e3e5f1` | Card and row separators |
| Accent | `--color-accent` | `#7cc4ff` | `#587cc7` | Links, selected state, primary action, and focus |
| Accent hover | `--color-accent-hover` | `#a5d7ff` | `#4368b3` | Hover and pressed emphasis |
| Accent soft | `--color-accent-soft` | `#132b45` | `#e5e9f8` | Selected and informational backgrounds |
| Success | `--color-success` | `#73d6a2` | `#237a57` | Published and saved states |
| Success soft | `--color-success-soft` | `#10291e` | `#e0f5eb` | Success background |
| Warning | `--color-warning` | `#f1c75b` | `#946200` | Draft and pending states |
| Warning soft | `--color-warning-soft` | `#2c2511` | `#fff3cf` | Warning background |
| Error | `--color-error` | `#ff8f85` | `#b94747` | Failure and destructive action |
| Error soft | `--color-error-soft` | `#31191d` | `#ffe8e8` | Error background |
| Overlay | `--color-overlay` | `rgb(2 6 18 / 72%)` | `rgb(41 57 86 / 34%)` | Modal veil and semantic overlay |
| Hero text | `--color-hero-text` | `#f3f6fc` | `#0f1c31` | Hero primary copy |
| Hero text muted | `--color-hero-text-muted` | `#b6c1d4` | `#223957` | Hero metadata |
| Hero border | `--color-hero-border` | `rgb(243 246 252 / 32%)` | `rgb(255 255 255 / 72%)` | Translucent Hero control edge |
| Hero surface | `--color-hero-surface` | `rgb(13 22 40 / 68%)` | `rgb(255 255 255 / 62%)` | Default translucent Hero control |
| Hero surface hover | `--color-hero-surface-hover` | `rgb(26 41 66 / 82%)` | `rgb(255 255 255 / 82%)` | Hovered translucent Hero control |
| Hero surface active | `--color-hero-surface-active` | `rgb(43 58 85 / 88%)` | `rgb(229 233 248 / 90%)` | Pressed translucent Hero control |
| Focus ring | `--color-focus-ring` | `rgb(124 196 255 / 38%)` | `rgb(88 124 199 / 34%)` | Outer keyboard focus halo |

Status color must always be paired with text or an icon. Hero text may only appear where the overlay maintains WCAG AA contrast.

## 3. Typography

The CJK-first `--font-family-display` stack is `"Noto Serif SC", "Songti SC", STSong, serif` for display text and the `--font-family-body` stack is `-apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif` for interface and body copy. No remote font is required. The serif stack supplies editorial character while local fallbacks avoid missing glyphs and render blocking.

| Token | Size / line-height | Weight | Use |
|---|---|---:|---|
| `--font-size-display` / `--line-height-display` | `56px / 1.18` | 700 | Hero title |
| `--font-size-h1` / `--line-height-h1` | `36px / 1.3` | 700 | Page title |
| `--font-size-h2` / `--line-height-h2` | `28px / 1.4` | 650 | Section heading |
| `--font-size-h3` / `--line-height-h3` | `20px / 1.5` | 600 | Card title |
| `--font-size-body` / `--line-height-body` | `16px / 1.8` | 400 | CJK article and interface body |
| `--font-size-small` / `--line-height-small` | `14px / 1.6` | 400 | Metadata |
| `--font-size-caption` / `--line-height-caption` | `12px / 1.5` | 600 | Compact labels |

Use `text-wrap: balance` on display headings and `text-wrap: pretty` on paragraphs. Do not split a CJK punctuation mark from its phrase, leave a single character on a heading line, or force article text into a narrow measure. Long-form detail reading measure is `50rem` (800px), paired with the existing desktop catalog without overlap.

## 4. Spacing, geometry, and depth

Spacing follows a 4px base unit: `--space-1: 4px`, `--space-2: 8px`, `--space-3: 12px`, `--space-4: 16px`, `--space-5: 20px`, `--space-6: 24px`, `--space-8: 32px`, `--space-10: 40px`, `--space-12: 48px`, `--space-16: 64px`, `--space-20: 80px`, and `--space-24: 96px`.

- Desktop support floor: `--layout-desktop-min: 1024px`.
- Main container: `--layout-container: 1500px` with `--layout-gutter: 24px`; viewport gutters continue to cap the rendered width without horizontal overflow.
- Hero: `--layout-hero-height: 100dvh`; the public Hero is exactly one dynamic viewport high with no lower-edge reservation.
- Overlay navigation: `--layout-nav-height: 72px`, transparent over the Hero and fixed with the opaque surface reading state after the Hero.
- Content grid: `--layout-sidebar: 350px`, `--layout-column-gap: 32px`; main stream consumes the remaining width.
- Card grid: exactly 3 columns with `--layout-card-gap: 24px` on supported desktop widths.
- Reading measure: `--layout-reading-measure: 50rem`.

Radii are `--radius-control: 8px`, `--radius-card: 14px`, `--radius-panel: 20px`, and `--radius-round: 9999px`. Each theme defines the same cool depth token set: `--shadow-card`, `--shadow-card-hover`, `--shadow-nav`, `--shadow-dialog`, and `--shadow-switcher`. Components never reconstruct or tint these shadows locally. Control lift and press use `--interaction-lift-control: -2px` and `--interaction-scale-pressed: 0.96`; the floating control layer is `--layer-floating-control: 60`.

## 5. Layout domains and primitives

The application root must declare one layout domain. Shared semantic primitives live on `:root`; each domain may override only what its product surface needs.

### `data-layout="public"`

Owns the public hero, overlay/sticky navigation, scene entry, `1500px` shell, `350px` sidebar, article stream, and 3-column card grid. Public cards use image, category, CJK title, excerpt, metadata, and a real destination link. Hover lifts the card and changes the interactive title to accent; keyboard focus receives the same information without requiring hover.

### Homepage search

The homepage keeps the exact `100dvh` Hero and both existing themes. Hero and content stay mounted but only one is displayed at a time; there is no stacked scrolling boundary or transition frame containing both. Downward wheel intent outside forms, controls and dialogs, or the labelled entry button, opens content. The remainder of that wheel gesture is consumed until it becomes idle. Content scroll never returns to the Hero; only the Header's `主页` action does. The Header contains the identity, Home and login/management entries, never a search form.

`HeroSearch` is a controlled search primitive. The Hero and right-rail card share mode, keyword and question drafts, result limit, and existing task identity. The rail variant stacks its mode controls, labelled field, and full-width action inside one surface; AI uses a multiline field and inline disclosure options. Hero entry, history, and task lifecycles retain their existing behavior.

The homepage reading canvas uses three rails: author and recommendations on the left, a shared article-results presentation in the center, and search followed by category and tag cards on the right. It retains document scrolling; only expanded taxonomy option lists have bounded internal scrolling. AI mode hides ordinary filter cards without discarding their selections.

Search fields retain visible labels. The rail uses ordinary surface tokens; the Hero uses its atmospheric tokens. The fixed theme switcher remains outside the reading center. Ordinary article counts and public AI status occupy a small secondary-text row below the article-section heading, never above it.

Public AI search reuses the ordinary single-column article cards. A small state-driven spinner and short sentence under the heading indicate submission, execution, or article loading; the same row then shows the number of available matches and the submitted topic. Reduced motion removes rotation while preserving the icon and text. Task ids, connection diagnostics, activity logs, evidence excerpts, and long generated conclusions are not public result UI. Necessary cancellation, retry, empty-result and failure feedback remain lightweight; existing confirmation dialogs and task lifecycle rules remain intact.

Recovery actions stay in that secondary row: uncertain cancellation offers a snapshot query separately from replaying cancellation, and restored cooldowns retain a manual reconnect action. Identity restart and expired-creation confirmation remain reachable even when an older Run is selected. Article-read retry requests only failed matches and preserves already readable cards within that Run; a new Run or identity cannot reuse those cards.

AI matches retain source order and use the existing public article endpoint for actual card metadata; non-public or unavailable articles are not fabricated from citations. Result topics are associated with the accepted Run, not the editable input draft. Requests completing after a Run or identity change cannot replace current results. `ArticleSection` supplies a description slot below its unique heading and an empty-content slot so normal and AI states share the same hierarchy.

### Homepage content rails

- The content scene overrides only its shell geometry: `--layout-container: 1500px`, `--layout-sidebar: 220px`, `--layout-filter-sidebar: 280px`, and `--layout-column-gap: 24px`. The middle column is `minmax(0, 1fr)`. The grid reserves the missing portion of the existing 80px right utility gutter so the fixed theme switcher cannot cover filter controls. Hero and other public routes retain their existing geometry. The desktop support boundary remains 1024px.
- Homepage article sections use the list variant: full-width summaries, optional small category artwork at the trailing edge, and no missing-image placeholder. Existing grid consumers keep their original cards.
- `ArticleFilterCard` represents one single-select category or tag filter. Categories use two columns; tags wrap. The selected option is first, receives an accent wash, and has a separate labelled cancel button. Options apply immediately and preserve the applied keyword and other filter. Ordinary keyword submission preserves both selections. Groups remain fixed AND; there is no multi-select or operator control.
- Categories initially show six options, tags eight. Each card can expand all options in a bounded list and collapse explicitly; there is no taxonomy search field. Selected options remain visible when collapsed. Long names wrap or truncate with a full-name title, never widen the rails.
- All controls retain visible keyboard focus and a 44px minimum hit area. Selecting or cancelling restores focus to the activated option after reordering, or a visible option if the cancelled item leaves the collapsed list. Unrelated route changes do not move focus. Passive support cards do not lift while their controls are being used.

The complete search primitive reserves a stable vertical slot for both modes, so switching does not move the mode control. Escape closes search options before the compact editor and returns focus to its trigger. Search actions use token-driven hover and press feedback, with opacity rather than movement under reduced motion. Public cancellation and abandoning an expired unknown operation use the existing `AppModal` with explicit outcome labels; they do not use native browser confirmation dialogs. Its `closed` notification follows the leave transition; public callers then restore focus to the opener, or to the result heading/search input when the opener is no longer available.

### `data-layout="author"`

Owns authenticated writing surfaces while retaining shared colors, spacing, focus, and status semantics. Editor states are explicit:

- **clean**: neutral saved timestamp.
- **dirty**: warning text plus visible save action.
- **saving**: `aria-busy="true"`, stable control width, and text progress.
- **saved**: success text announced through a polite live region.
- **error**: error text with retry action; draft content remains intact.

Author surfaces may use `--color-editor-canvas` and `--color-editor-surface` only as aliases of the shared canvas and surface tokens, plus `--layout-editor-measure`; they must not retain raw light values, redefine the public accent, or invent button colors.

### `data-layout="admin"`

Reserved as an integration boundary only. It currently defines no concrete product styling, colors, dimensions, navigation, or content model. A later task must introduce its own documented contract before adding values to this scope.

### Shared primitives

- **BlogHero**: semantic Hero section with centered readable copy and an exact `100dvh` boundary. `BlogLayout` renders no theme-specific image node or branch. `main.css` composes `--hero-background-color`, `--hero-background-image`, `--hero-background-position`, and `--hero-overlay`; night supplies `/images/hero-starry.jpg`, while sky supplies only layered gradients. Hero control, border, and text tokens remain theme-owned.
- **OverlayNav**: real navigation landmark; transparent with Hero text only while the Hero covers the full `72px` Header band. Once the Hero enters that band, its reading state uses `surface`, `text-primary`, and `border-subtle` with `BlogShell` alignment. The active link uses text plus a visual marker.
- **Scene entry**: a hashless `type="button"` control centered at the Hero bottom, labelled `进入内容区`, with a decorative Heroicons entry icon and a minimum `44px` target. It swaps scenes rather than scrolling or looping an arrow. The skip link also opens content and focuses its main landmark. Returning through `主页` restores the Hero while preserving search and result state. Content re-entry restores its reading position; a new search starts at content top. Only the newly displayed scene may fade in, with no overlapping exit or vertical slide.
- **BlogShell**: centered `1500px` container capped by viewport gutters.
- **BlogContentGrid**: article stream plus `350px` sidebar.
- **BlogCardGrid**: three editorial cards per row at the supported desktop width.
- **BlogCard**: linked article summary with hover, active, focus-visible, and visited-safe semantics.
- **AuthorProfile**: uses `/images/avatar.jpg`; includes textual author identity so the portrait is not the only identifier.
- **EditorSurface**: exposes clean, dirty, saving, saved, and error states without layout shift.
- **ArticleTypeDropdown**: a controlled combobox whose popup pins `新增分类` as the first action, followed by versioned real category options and separate native delete buttons in a right-side destructive-action column. The trigger, options, and delete controls are at least `44px` high. Option navigation keeps DOM focus on the trigger and exposes virtual focus through `aria-activedescendant`; `Tab` can reach each native delete button. `Escape` closes the popup and restores trigger focus. Any create or delete mutation disables the full control, and deletion always opens a modal confirmation before the versioned request is sent.
- **ArticleSettings**: controlled tags remain wrapping checkbox-and-name options; `新增标签` uses `PlusIcon`, while each sibling delete button uses `XCircleIcon` and emits the complete versioned tag without changing selection. Create, checkbox, and delete controls keep the shared `44px` target and lock together when disabled.
- **MarkdownSurface**: `.blog-markdown` is the stable wrapper for preview and catalog. Heading, body, link, inline code, preformatted code, blockquote, table, and rule colors are global `main.css` rules using only semantic tokens; render components never define a local Markdown palette.
- **Shared controls and feedback**: the referenced dialog and toast primitives continue to consume shared semantic tokens; obsolete demo-only table, filter, status, breadcrumb, alert, button, and page-header primitives are not part of the runtime surface.
- **ThemeSwitcher**: mounted once beside `AppToast` at the application root so routing cannot destroy it. The fixed `48px × 48px` circular button sits `24px` from the bottom and right, uses Heroicons only, and describes the destination theme through matching Chinese `aria-label` and `title`. Night displays `SunIcon` to offer sky; sky displays `MoonIcon` to offer night. Surface, border, icon, focus, shadow, hover lift, and press scale all consume semantic tokens.

## 6. Interaction and motion

Motion tokens are `--duration-fast: 150ms`, `--duration-standard: 220ms`, `--duration-slow: 360ms`, `--duration-scroll-cue: 1.6s`, `--duration-reduced: 0.01ms`, and `--ease-standard: cubic-bezier(0.2, 0.8, 0.2, 1)`. Border and keyboard-focus geometry use `--border-width: 1px`, `--focus-outline-width: 2px`, `--focus-outline-offset: 2px`, and `--focus-halo-width: 4px`.

- Hover changes color, border, shadow, opacity, or `transform`; it never changes layout dimensions.
- Active controls use a small transform to confirm the press.
- `:focus-visible` uses a 2px accent outline and a focus halo with at least 3:1 adjacent contrast.
- Navigation settling and card lift animate only `transform`, `opacity`, `filter`, and shadow.
- Homepage scene entry performs an exclusive replacement and a short opacity arrival; reduced motion displays the new scene immediately. No permanent entry-icon animation remains.
- Document reload restoration is application-owned and one-shot: every `pagehide` stores the exact full path plus either the pixel position or bottom intent in `sessionStorage`; an unfinished reload preserves its original semantic target across another refresh. Only exact-path reloads of Home, Article List, and Article Detail consume a target. Their first asynchronous terminal state starts a bounded visual-stability gate: Vue rendering settles, current document images reach load/error, and document height remains quiet for at least `200ms`; bottom restoration then uses the final `scrollHeight - innerHeight`. Initial reload restoration takes precedence over Vue Router's initial history-state position, while later back/forward navigation still prioritizes `savedPosition`. Any later SPA navigation, user scroll intent, path mismatch, or the shared five-second timeout permanently cancels pending restoration and releases temporary observers, listeners, and timers. Restoration itself is immediate; only user-invoked ScrollCue movement may be smooth.
- `prefers-reduced-motion: reduce` removes nonessential animation, smooth scrolling, and transforms while preserving instant state changes.
- The star field is a static image. Continuous canvas animation, random button colors, and purposeless decorative looping motion are prohibited.
- Theme initialization is synchronous and precedes CSS/module loading in `index.html`: strictly parsed localStorage value `scyg-blog-theme` wins, otherwise `prefers-color-scheme: dark` selects night and the light preference selects sky. The root night selector remains the CSS fallback when JavaScript is unavailable.
- `src/theme/theme.ts` owns one module-level reactive theme state and toggle action. Application startup reapplies the same preference before Vue mounts, each explicit toggle is persisted when storage is available, and `meta[name="theme-color"]` is updated from the active theme's `--color-browser-theme` computed value rather than a TypeScript color literal.

## 7. Accessibility and content constraints

- Target WCAG 2.2 AA: 4.5:1 normal text, 3:1 large text and controls.
- A skip link is the first keyboard destination on public and author layouts.
- All interactive elements have visible hover and focus states; DOM and visual reading order remain identical.
- Controls are at least 44 x 44px even though this phase supports desktop only.
- Scene entry keeps its minimum `44px` target, visible Chinese label and hashless button semantics; the skip link opens and focuses content rather than scrolling a hidden section.
- Content images require dimensions to prevent layout shift. The night Hero image is a decorative CSS background backed by equivalent heading and context; author portraits use useful alt text.
- CJK headings use balanced wrapping; body text uses pretty wrapping and generous line height. Do not justify CJK body text.
- Section and main anchors use the navigation height as scroll margin so keyboard focus and route hashes stay below the fixed Header; Vue Router consumes that computed margin explicitly. Scene entry removes the Hero from layout, while `.public-content` keeps top padding below navigation.
- Reduced-motion users receive the same state information without movement.
- Theme switching remains a native keyboard-operable button with a visible semantic focus ring. Its hover lift and active scale are removed by the global reduced-motion rule while icon and accessible-name state changes remain immediate.

## 8. Accepted accessibility debt and exit criteria

| Debt | Affected users | Reason accepted in T1 | Exit criterion |
|---|---|---|---|
| Public and author DOM do not yet consume the new primitives | Keyboard, screen-reader, and low-vision users cannot exercise the future flows yet | T1 changes only the contract, global CSS, and project-owned assets | Implementing tasks must add landmarks, skip links, labels, and test each key flow |
| Desktop-only width floor may require horizontal scrolling below `1024px` | Narrow-window and zoom users | The approved phase explicitly excludes mobile and tablet behavior | A separately approved responsive phase defines breakpoints and revalidates 200% zoom |
| Local CJK serif appearance varies by operating system | Readers on systems without the preferred local serif | No new dependency or remote font is allowed | Validate target deployment fonts or add an approved self-hosted CJK subset |

This theme architecture is accepted when static token-set equality, raw-color location, import order, component palette, and diff-whitespace checks pass. Interactive author changes require browser validation; the earlier static-only historical scope no longer applies to them.
