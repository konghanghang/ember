# Ember Design System (v1.0)

> **Design Philosophy**: "Netflix Light" — A cinematic, high-impact aesthetic adapted for a clean, bright user environment. It combines the boldness of media streaming platforms with the clarity of modern SaaS tools.

## 1. Core Visual Identity

### Color Palette

**Primary: Ember Red** (Netflix Inspired)
- High energy, passionate, cinematic.
- Used for primary actions (Buttons), highlights, and brand identity.

| Token | Hex | Tailwind Class | Usage |
|-------|-----|----------------|-------|
| **Default** | `#E50914` | `bg-ember` / `text-ember` | Primary buttons, active states, logo |
| **Glow** | `#F40612` | `bg-ember-glow` | Hover states, glowing effects |
| **Dim** | `rgba(229, 9, 20, 0.1)` | `bg-ember/10` | Subtle backgrounds, decorative elements |

**Foundations: Modern Clean**
- Clean, bright, distraction-free canvas.

| Token | Hex | Tailwind Class | Usage |
|-------|-----|----------------|-------|
| **Background** | `#F9FAFB` | `bg-cinema-bg` | Page background |
| **Surface** | `#FFFFFF` | `bg-cinema-surface` | Cards, panels, navigation |
| **Text Primary** | `#111827` | `text-text-primary` | Headings, main content |
| **Text Secondary** | `#6B7280` | `text-text-secondary` | Descriptions, metadata |

---

## 2. Typography

**Font Family**: System Sans-Serif stack (Inter, San Francisco, Segoe UI) optimized for legibility.

- **Headings**: Bold, tight tracking (`tracking-tight`).
- **Labels/Nav**: Uppercase, wide tracking (`tracking-wide`), smaller size.
- **Body**: Regular weight, relaxed line height.

---

## 3. Component Styles

### Buttons (`.btn-ember`)
A vibrant, slightly glossy button style that mimics a "Play" button.

```css
.btn-ember {
  background: linear-gradient(135deg, var(--ember-red), var(--ember-glow));
  color: white;
  box-shadow: 0 4px 14px 0 rgba(229, 9, 20, 0.39);
  transition: all 0.3s ease;
}

.btn-ember:hover {
  transform: translateY(-1px);
  box-shadow: 0 6px 20px rgba(229, 9, 20, 0.23);
  filter: brightness(1.1);
}
```

### Cards (`.panel-clean`)
Clean white surfaces with very soft, diffused shadows to create depth without heaviness.

```css
.panel-clean {
  background: white;
  border: 1px solid rgba(0, 0, 0, 0.05);
  box-shadow: 0 10px 40px -10px rgba(0,0,0,0.05); /* Soft diffused shadow */
  border-radius: 1rem; /* 16px */
}
```

### Inputs (`.input-ember`)
Minimalist inputs that glow with the brand color when focused.

```css
.input-ember:focus {
  border-color: var(--ember-red);
  box-shadow: 0 0 0 4px rgba(229, 9, 20, 0.1); /* Red Halo */
}
```

---

## 4. Layout Patterns

### Hero Section (Billboard)
- **Concept**: Maximum impact, minimal noise.
- **Structure**: Centered, large typography, single clear Call-to-Action.
- **Background**: Subtle gradients or blurs, never solid blocks of heavy color.

### Feature Grids
- **Style**: 3-column layouts using `.panel-clean` cards.
- **Interaction**: Slight lift (`translate-y`) on hover to encourage clickability.

### Footer
- **Style**: Light (`bg-gray-50`), unobtrusive.
- **Purpose**: A quiet ending to the page, blending seamlessly with the content.

---

## 5. Usage Guidelines

- **Do** use `bg-ember` sparingly for high-priority actions.
- **Do not** use large blocks of red background (it's too aggressive).
- **Do** keep the background `#F9FAFB` to maintain the "Light" aesthetic.
- **Do** ensure all text on red backgrounds is white (`#FFFFFF`).
