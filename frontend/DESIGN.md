# SCYG Blog Desktop Design System

## 0. Research and scope

- Baseline: the existing semantic primitive names remain stable across two complete themes. `night` preserves the approved deep-night surface, while `sky` expresses the same hierarchy through clear blue atmosphere and white cloud surfaces.
- Theme ownership: `src/assets/themes/shared.css` owns only theme-independent typography, spacing, geometry, motion, and layout tokens. `night.css` and `sky.css` own identical color, shadow, browser chrome, and Hero token sets. Raw colors are allowed only in `src/assets/themes/*.css` and this document; `main.css`, Tailwind configuration, TypeScript, and Vue components consume semantic variables only.
- Visual source: `night` uses the project-owned starry welcome image; `sky` uses layered CSS gradients and requires no additional asset. The project-owned portrait supplies author identity. No external logo, copy, or proprietary visual asset is part of this system.
- Layout grammar: an exact `100dvh` atmospheric Hero, transparent overlay navigation that becomes a solid theme surface, a bottom-centered ScrollCue leading into the reading canvas, and a constrained editorial grid.
- Support boundary: desktop only. The validated target is Microsoft Edge at 1440 x 900. Widths below `1024px` are outside this phase and must not create new mobile or tablet requirements.

## 1. Identity and principles

The public blog supports two coherent atmospheric readings. `night` moves from a photographic star field into a near-black navy reading canvas; `sky` moves from a layered clear-sky and soft-cloud Hero into an airy pale-blue canvas with white reading surfaces. Both preserve the same editorial hierarchy, restrained ScrollCue handoff, semantic accent usage, component geometry, and interaction behavior.

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

Owns the public hero, overlay/sticky navigation, ScrollCue, `1500px` shell, `350px` sidebar, article stream, and 3-column card grid. Public cards use image, category, CJK title, excerpt, metadata, and a real destination link. Hover lifts the card and changes the interactive title to accent; keyboard focus receives the same information without requiring hover.

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
- **ScrollCue**: a hashless `type="button"` control centered at the Hero bottom with a minimum `44px × 44px` target, the Chinese accessible name `向下浏览文章`, and a decorative `ChevronDownIcon` from Heroicons marked `aria-hidden="true"`. It uses `scrollIntoView({ block: "start" })` to align `#blog-content` exactly with the viewport top without changing the URL; smooth behavior applies normally and reduced motion is immediate. The separate skip link remains a native `href="#blog-content"` anchor. Its translucent surface, border, text, hover, focus-visible, and active colors all come from Hero semantic tokens. Only the inner icon loops at about `1.6s`, using `transform` and `opacity`.
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
- ScrollCue animates only its inner directional icon with `transform` and `opacity`; its button scrolls to `#blog-content` without writing a hash and explicitly honors reduced motion.
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
- ScrollCue keeps its `44px × 44px` target, Chinese accessible name, hashless button semantics, and immediate movement when reduced motion is requested.
- Content images require dimensions to prevent layout shift. The night Hero image is a decorative CSS background backed by equivalent heading and context; author portraits use useful alt text.
- CJK headings use balanced wrapping; body text uses pretty wrapping and generous line height. Do not justify CJK body text.
- General section and main anchors use the navigation height as scroll margin. `main#blog-content` is the deliberate exception: it has zero scroll margin so ScrollCue removes the Hero exactly, while `.public-content` keeps its own top padding to avoid the fixed Header.
- Reduced-motion users receive the same state information without movement.
- Theme switching remains a native keyboard-operable button with a visible semantic focus ring. Its hover lift and active scale are removed by the global reduced-motion rule while icon and accessible-name state changes remain immediate.

## 8. Accepted accessibility debt and exit criteria

| Debt | Affected users | Reason accepted in T1 | Exit criterion |
|---|---|---|---|
| Public and author DOM do not yet consume the new primitives | Keyboard, screen-reader, and low-vision users cannot exercise the future flows yet | T1 changes only the contract, global CSS, and project-owned assets | Implementing tasks must add landmarks, skip links, labels, and test each key flow |
| Desktop-only width floor may require horizontal scrolling below `1024px` | Narrow-window and zoom users | The approved phase explicitly excludes mobile and tablet behavior | A separately approved responsive phase defines breakpoints and revalidates 200% zoom |
| Local CJK serif appearance varies by operating system | Readers on systems without the preferred local serif | No new dependency or remote font is allowed | Validate target deployment fonts or add an approved self-hosted CJK subset |

This theme architecture is accepted when static token-set equality, raw-color location, import order, component palette, and diff-whitespace checks pass. Interactive author changes require browser validation; the earlier static-only historical scope no longer applies to them.
