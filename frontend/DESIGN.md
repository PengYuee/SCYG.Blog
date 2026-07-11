# SCYG Blog Desktop Design System

## 0. Research and scope

- Baseline: the replaced contract described a blue-accent operations console. This contract intentionally removes that product language while preserving semantic primitive names consumed by existing Vue components.
- Visual source: the project-owned starry welcome image supplies the hero atmosphere, and the project-owned portrait supplies the author identity. No external logo, copy, or proprietary visual asset is part of this system.
- Layout grammar: a full-viewport image hero, transparent overlay navigation that becomes a solid sticky reading aid, a wave-shaped handoff into a pale canvas, and a constrained editorial grid.
- Support boundary: desktop only. The validated target is Microsoft Edge at 1440 x 900. Widths below `1024px` are outside this phase and must not create new mobile or tablet requirements.

## 1. Identity and principles

The public blog should feel like looking from a quiet night sky into a bright reading room. The memorable transition is the soft wave where the photographic `100vh` Hero gives way to the `#f7f9fe` editorial canvas. Coral `#ff623e` is deliberately scarce: it marks links, selected states, focus, and primary actions instead of decorating every surface.

Three rules govern the surface:

1. Content remains the focal point. Photography creates atmosphere; white surfaces and warm-neutral text support long CJK reading.
2. Depth is restrained and directional. Cards lift only on interaction; the sticky navigation uses opacity and blur to separate itself from the hero.
3. Motion communicates state. Hover, focus, navigation settling, and editor save states may transition; there is no continuous canvas or decorative looping animation.

## 2. Semantic color system

Every CSS and Tailwind semantic color maps to this table. Raw visual colors outside this contract are not allowed.

| Role | CSS token | Value | Use |
|---|---|---:|---|
| Canvas | `--color-canvas` | `#f7f9fe` | Public reading background |
| Surface | `--color-surface` | `#ffffff` | Cards, panels, sticky navigation |
| Surface muted | `--color-surface-muted` | `#f0f3fa` | Metadata bands and quiet controls |
| Surface hover | `--color-surface-hover` | `#fff2ed` | Interactive surface hover |
| Text primary | `--color-text-primary` | `#263044` | Titles and body copy |
| Text secondary | `--color-text-secondary` | `#657087` | Summaries and metadata |
| Text tertiary | `--color-text-tertiary` | `#8f98aa` | Placeholders and timestamps |
| Border | `--color-border` | `#dce2ed` | Controls and strong divisions |
| Border subtle | `--color-border-subtle` | `#e9edf5` | Card and row separators |
| Accent | `--color-accent` | `#ff623e` | Links, selected state, primary action, focus |
| Accent hover | `--color-accent-hover` | `#e64d2c` | Hover and pressed emphasis |
| Accent soft | `--color-accent-soft` | `#fff0eb` | Selected and informational backgrounds |
| Success | `--color-success` | `#18794e` | Published and saved states |
| Success soft | `--color-success-soft` | `#e9f7f0` | Success background |
| Warning | `--color-warning` | `#946200` | Draft and pending states |
| Warning soft | `--color-warning-soft` | `#fff5d6` | Warning background |
| Error | `--color-error` | `#c4320a` | Failure and destructive action |
| Error soft | `--color-error-soft` | `#fff0eb` | Error background |
| Overlay | `--color-overlay` | `rgb(12 20 39 / 56%)` | Hero readability and modal veil |
| Hero text | `--color-hero-text` | `#ffffff` | Copy over the star field |
| Hero text muted | `--color-hero-text-muted` | `rgb(255 255 255 / 78%)` | Hero metadata |
| Focus ring | `--color-focus-ring` | `rgb(255 98 62 / 34%)` | Outer keyboard focus halo |

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
- Main container: `--layout-container: 1220px` with `--layout-gutter: 24px`.
- Hero: `--layout-hero-height: 100vh`; implementation also uses `100dvh` as the stable viewport enhancement.
- Overlay navigation: `--layout-nav-height: 72px`, transparent over the hero and sticky with a solid surface after the hero.
- Wave transition: `--layout-wave-height: 72px`, visually joining hero and canvas without continuous animation.
- Content grid: `--layout-sidebar: 300px`, `--layout-column-gap: 32px`; main stream consumes the remaining width.
- Card grid: exactly 3 columns with `--layout-card-gap: 24px` on supported desktop widths.
- Reading measure: `--layout-reading-measure: 50rem`.

Radii are `--radius-control: 8px`, `--radius-card: 14px`, `--radius-panel: 20px`, and `--radius-round: 9999px`. Shadows are `--shadow-card: 0 12px 32px rgb(40 55 82 / 10%)`, `--shadow-card-hover: 0 18px 42px rgb(40 55 82 / 16%)`, `--shadow-nav: 0 8px 24px rgb(40 55 82 / 10%)`, and `--shadow-dialog: 0 20px 48px rgb(40 55 82 / 18%)`.

## 5. Layout domains and primitives

The application root must declare one layout domain. Shared semantic primitives live on `:root`; each domain may override only what its product surface needs.

### `data-layout="public"`

Owns the public hero, overlay/sticky navigation, wave, `1220px` shell, `300px` sidebar, article stream, and 3-column card grid. Public cards use image, category, CJK title, excerpt, metadata, and a real destination link. Hover lifts the card and changes the interactive title to accent; keyboard focus receives the same information without requiring hover.

### `data-layout="author"`

Owns authenticated writing surfaces while retaining shared colors, spacing, focus, and status semantics. Editor states are explicit:

- **clean**: neutral saved timestamp.
- **dirty**: warning text plus visible save action.
- **saving**: `aria-busy="true"`, stable control width, and text progress.
- **saved**: success text announced through a polite live region.
- **error**: error text with retry action; draft content remains intact.

Author surfaces may use `--color-editor-canvas`, `--color-editor-surface`, and `--layout-editor-measure`; they must not redefine the public accent or invent button colors.

### `data-layout="admin"`

Reserved as an integration boundary only. It currently defines no concrete product styling, colors, dimensions, navigation, or content model. A later task must introduce its own documented contract before adding values to this scope.

### Shared primitives

- **BlogHero**: semantic hero section, meaningful image alternative, centered readable copy, no continuous animation.
- **OverlayNav**: real navigation landmark; transparent hero state and sticky solid state; active link uses text plus a visual marker.
- **WaveTransition**: decorative separator hidden from assistive technology.
- **BlogShell**: centered `1220px` container.
- **BlogContentGrid**: article stream plus `300px` sidebar.
- **BlogCardGrid**: three editorial cards per row at the supported desktop width.
- **BlogCard**: linked article summary with hover, active, focus-visible, and visited-safe semantics.
- **AuthorProfile**: uses `/images/avatar.jpg`; includes textual author identity so the portrait is not the only identifier.
- **EditorSurface**: exposes clean, dirty, saving, saved, and error states without layout shift.
- **Shared controls and feedback**: the referenced dialog and toast primitives continue to consume shared semantic tokens; obsolete demo-only table, filter, status, breadcrumb, alert, button, and page-header primitives are not part of the runtime surface.

## 6. Interaction and motion

Motion tokens are `--duration-fast: 150ms`, `--duration-standard: 220ms`, `--duration-slow: 360ms`, `--duration-reduced: 0.01ms`, and `--ease-standard: cubic-bezier(0.2, 0.8, 0.2, 1)`. Border and keyboard-focus geometry use `--border-width: 1px`, `--focus-outline-width: 2px`, `--focus-outline-offset: 2px`, and `--focus-halo-width: 4px`.

- Hover changes color, border, shadow, opacity, or `transform`; it never changes layout dimensions.
- Active controls use a small transform to confirm the press.
- `:focus-visible` uses a 2px accent outline and a focus halo with at least 3:1 adjacent contrast.
- Navigation settling and card lift animate only `transform`, `opacity`, `filter`, and shadow.
- `prefers-reduced-motion: reduce` removes nonessential animation, smooth scrolling, and transforms while preserving instant state changes.
- The star field is a static image. Continuous canvas animation, random button colors, and decorative looping motion are prohibited.

## 7. Accessibility and content constraints

- Target WCAG 2.2 AA: 4.5:1 normal text, 3:1 large text and controls.
- A skip link is the first keyboard destination on public and author layouts.
- All interactive elements have visible hover and focus states; DOM and visual reading order remain identical.
- Controls are at least 44 x 44px even though this phase supports desktop only.
- Images require dimensions to prevent layout shift. The hero image is decorative only when equivalent heading and context exist; author portraits use useful alt text.
- CJK headings use balanced wrapping; body text uses pretty wrapping and generous line height. Do not justify CJK body text.
- Sticky navigation must not cover anchor targets; sections use the navigation height as scroll margin.
- Reduced-motion users receive the same state information without movement.

## 8. Accepted accessibility debt and exit criteria

| Debt | Affected users | Reason accepted in T1 | Exit criterion |
|---|---|---|---|
| Public and author DOM do not yet consume the new primitives | Keyboard, screen-reader, and low-vision users cannot exercise the future flows yet | T1 changes only the contract, global CSS, and project-owned assets | Implementing tasks must add landmarks, skip links, labels, and test each key flow |
| Desktop-only width floor may require horizontal scrolling below `1024px` | Narrow-window and zoom users | The approved phase explicitly excludes mobile and tablet behavior | A separately approved responsive phase defines breakpoints and revalidates 200% zoom |
| Local CJK serif appearance varies by operating system | Readers on systems without the preferred local serif | No new dependency or remote font is allowed | Validate target deployment fonts or add an approved self-hosted CJK subset |

T1 is accepted only when token and forbidden-term checks pass, both copied assets are readable, a fresh Microsoft Edge 1440 x 900 artifact exists, and browser/server cleanup is recorded.
