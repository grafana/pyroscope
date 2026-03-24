import { useState } from 'react'
import './theme.css'

// ─── Theme toggle ────────────────────────────────────────────────────────────

function useTheme() {
  const [theme, setTheme] = useState<'dark' | 'light'>('dark')
  const toggle = () => {
    const next = theme === 'dark' ? 'light' : 'dark'
    setTheme(next)
    if (next === 'light') {
      document.documentElement.setAttribute('data-theme', 'light')
    } else {
      document.documentElement.removeAttribute('data-theme')
    }
  }
  return { theme, toggle }
}

// ─── Inline style helpers (keeps JSX readable, avoids a separate CSS file) ──

const s = {
  page: {
    minHeight: '100dvh',
    padding: 'var(--space-8)',
    background: 'var(--bg-canvas)',
  } as React.CSSProperties,

  header: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    marginBottom: 'var(--space-8)',
    paddingBottom: 'var(--space-4)',
    borderBottom: '1px solid var(--border-medium)',
  } as React.CSSProperties,

  title: {
    fontSize: 'var(--text-3xl)',
    fontWeight: 'var(--weight-bold)',
    color: 'var(--text-primary)',
    letterSpacing: 'var(--tracking-tight)',
  } as React.CSSProperties,

  subtitle: {
    fontSize: 'var(--text-sm)',
    color: 'var(--text-secondary)',
    marginTop: 'var(--space-1)',
  } as React.CSSProperties,

  grid: {
    display: 'grid',
    gridTemplateColumns: 'repeat(auto-fill, minmax(340px, 1fr))',
    gap: 'var(--space-6)',
  } as React.CSSProperties,

  card: {
    background: 'var(--bg-primary)',
    border: '1px solid var(--border-medium)',
    borderRadius: 'var(--radius-lg)',
    padding: 'var(--space-5)',
    boxShadow: 'var(--shadow-sm)',
  } as React.CSSProperties,

  cardTitle: {
    fontSize: 'var(--text-xs)',
    fontWeight: 'var(--weight-medium)',
    color: 'var(--text-secondary)',
    letterSpacing: 'var(--tracking-caps)',
    textTransform: 'uppercase' as const,
    marginBottom: 'var(--space-4)',
  },

  row: {
    display: 'flex',
    flexWrap: 'wrap' as const,
    gap: 'var(--space-2)',
    alignItems: 'center',
  } as React.CSSProperties,

  col: {
    display: 'flex',
    flexDirection: 'column' as const,
    gap: 'var(--space-2)',
  } as React.CSSProperties,

  divider: {
    height: '1px',
    background: 'var(--border-weak)',
    margin: 'var(--space-4) 0',
  } as React.CSSProperties,

  label: {
    fontSize: 'var(--text-xs)',
    color: 'var(--text-secondary)',
  } as React.CSSProperties,
}

// ─── Small reusable demo components ──────────────────────────────────────────

type ButtonVariant = 'primary' | 'secondary' | 'success' | 'error' | 'warning' | 'ghost'

function Button({
  children,
  variant = 'secondary',
  size = 'md',
  disabled,
}: {
  children: React.ReactNode
  variant?: ButtonVariant
  size?: 'sm' | 'md' | 'lg'
  disabled?: boolean
}) {
  const variantStyles: Record<ButtonVariant, React.CSSProperties> = {
    primary: {
      background: 'var(--color-primary)',
      color: 'var(--color-primary-foreground)',
      border: '1px solid transparent',
    },
    secondary: {
      background: 'var(--color-secondary)',
      color: 'var(--text-primary)',
      border: '1px solid var(--color-secondary-border)',
    },
    success: {
      background: 'var(--color-success)',
      color: 'var(--color-success-foreground)',
      border: '1px solid transparent',
    },
    error: {
      background: 'var(--color-error)',
      color: 'var(--color-error-foreground)',
      border: '1px solid transparent',
    },
    warning: {
      background: 'var(--color-warning)',
      color: 'var(--color-warning-foreground)',
      border: '1px solid transparent',
    },
    ghost: {
      background: 'transparent',
      color: 'var(--text-primary)',
      border: '1px solid var(--border-medium)',
    },
  }

  const sizeStyles = {
    sm: { padding: 'var(--space-1) var(--space-3)', fontSize: 'var(--text-xs)' },
    md: { padding: 'var(--space-2) var(--space-4)', fontSize: 'var(--text-md)' },
    lg: { padding: 'var(--space-3) var(--space-6)', fontSize: 'var(--text-lg)' },
  }

  return (
    <button
      disabled={disabled}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 'var(--space-2)',
        borderRadius: 'var(--radius-md)',
        fontWeight: 'var(--weight-medium)',
        cursor: disabled ? 'not-allowed' : 'pointer',
        opacity: disabled ? 'var(--action-disabled-opacity, 0.4)' : 1,
        transition: `background var(--duration-base) var(--ease-out),
                     opacity var(--duration-base) var(--ease-out)`,
        ...variantStyles[variant],
        ...sizeStyles[size],
      }}
    >
      {children}
    </button>
  )
}

function Badge({
  children,
  variant = 'primary',
}: {
  children: React.ReactNode
  variant?: 'primary' | 'success' | 'error' | 'warning' | 'secondary'
}) {
  const styles: Record<string, React.CSSProperties> = {
    primary:   { background: 'var(--color-primary-subtle)',  color: 'var(--color-primary-text)',  border: '1px solid var(--color-primary-border)' },
    success:   { background: 'var(--color-success-subtle)',  color: 'var(--color-success-text)',  border: '1px solid var(--color-success-border)' },
    error:     { background: 'var(--color-error-subtle)',    color: 'var(--color-error-text)',    border: '1px solid var(--color-error-border)' },
    warning:   { background: 'var(--color-warning-subtle)',  color: 'var(--color-warning-text)',  border: '1px solid var(--color-warning-border)' },
    secondary: { background: 'var(--color-secondary)',       color: 'var(--text-secondary)',      border: '1px solid var(--color-secondary-border)' },
  }

  return (
    <span style={{
      display: 'inline-flex',
      alignItems: 'center',
      padding: 'var(--space-0-5) var(--space-2)',
      borderRadius: 'var(--radius-sm)',
      fontSize: 'var(--text-xs)',
      fontWeight: 'var(--weight-medium)',
      letterSpacing: 'var(--tracking-wide)',
      ...styles[variant],
    }}>
      {children}
    </span>
  )
}

function Input({ placeholder, disabled }: { placeholder?: string; disabled?: boolean }) {
  return (
    <input
      placeholder={placeholder}
      disabled={disabled}
      style={{
        background: 'var(--bg-secondary)',
        color: 'var(--text-primary)',
        border: '1px solid var(--border-medium)',
        borderRadius: 'var(--radius-md)',
        padding: 'var(--space-2) var(--space-3)',
        fontSize: 'var(--text-md)',
        width: '100%',
        outline: 'none',
        transition: `border-color var(--duration-base) var(--ease-out),
                     box-shadow var(--duration-base) var(--ease-out)`,
        opacity: disabled ? 0.5 : 1,
        cursor: disabled ? 'not-allowed' : 'text',
      }}
      onFocus={e => {
        e.currentTarget.style.borderColor = 'var(--color-primary-border)'
        e.currentTarget.style.boxShadow = '0 0 0 3px var(--action-focus)'
      }}
      onBlur={e => {
        e.currentTarget.style.borderColor = 'var(--border-medium)'
        e.currentTarget.style.boxShadow = 'none'
      }}
    />
  )
}

function Swatch({ token, value }: { token: string; value: string }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)' }}>
      <div style={{
        width: 28,
        height: 28,
        borderRadius: 'var(--radius-sm)',
        background: value,
        border: '1px solid var(--border-medium)',
        flexShrink: 0,
      }} />
      <div>
        <div style={{ fontSize: 'var(--text-xs)', color: 'var(--text-primary)', fontFamily: 'var(--font-mono)' }}>{token}</div>
        <div style={{ fontSize: 'var(--text-xs)', color: 'var(--text-secondary)', fontFamily: 'var(--font-mono)' }}>{value}</div>
      </div>
    </div>
  )
}

function SpacingSwatch({ token, size }: { token: string; size: string }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-3)' }}>
      <div style={{
        width: size,
        height: 12,
        background: 'var(--color-primary-subtle)',
        border: '1px solid var(--color-primary-border)',
        borderRadius: 2,
        flexShrink: 0,
        minWidth: 2,
      }} />
      <div style={{ fontSize: 'var(--text-xs)', fontFamily: 'var(--font-mono)', color: 'var(--text-secondary)' }}>
        {token} <span style={{ color: 'var(--text-disabled)' }}>= {size}</span>
      </div>
    </div>
  )
}

function ShadowSwatch({ token, shadow }: { token: string; shadow: string }) {
  return (
    <div style={{
      background: 'var(--bg-primary)',
      border: '1px solid var(--border-weak)',
      borderRadius: 'var(--radius-md)',
      padding: 'var(--space-3)',
      boxShadow: shadow,
    }}>
      <div style={{ fontSize: 'var(--text-xs)', fontFamily: 'var(--font-mono)', color: 'var(--text-secondary)' }}>{token}</div>
    </div>
  )
}

// ─── Section cards ─────────────────────────────────────────────────────────

function Card({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div style={s.card}>
      <div style={s.cardTitle}>{title}</div>
      {children}
    </div>
  )
}

// ─── App ─────────────────────────────────────────────────────────────────────

export default function App() {
  const { theme, toggle } = useTheme()

  return (
    <div style={s.page}>
      {/* Header */}
      <div style={s.header}>
        <div>
          <div style={s.title}>Design System</div>
          <div style={s.subtitle}>Pyroscope UI · theme.css kitchen sink</div>
        </div>
        <button
          onClick={toggle}
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: 'var(--space-2)',
            background: 'var(--bg-secondary)',
            color: 'var(--text-primary)',
            border: '1px solid var(--border-medium)',
            borderRadius: 'var(--radius-md)',
            padding: 'var(--space-2) var(--space-4)',
            fontSize: 'var(--text-md)',
            fontWeight: 'var(--weight-medium)',
            cursor: 'pointer',
            transition: 'background var(--duration-base) var(--ease-out)',
          }}
        >
          {theme === 'dark' ? '☀ Light' : '☾ Dark'}
        </button>
      </div>

      <div style={s.grid}>

        {/* ── Backgrounds ─────────────────────────────────────────────────── */}
        <Card title="Backgrounds">
          {[
            ['--bg-canvas',    'var(--bg-canvas)'],
            ['--bg-primary',   'var(--bg-primary)'],
            ['--bg-secondary', 'var(--bg-secondary)'],
            ['--bg-elevated',  'var(--bg-elevated)'],
          ].map(([token, val]) => (
            <div key={token} style={{ ...s.row, marginBottom: 'var(--space-2)' }}>
              <Swatch token={token} value={val} />
            </div>
          ))}
        </Card>

        {/* ── Borders ─────────────────────────────────────────────────────── */}
        <Card title="Borders">
          {[
            ['--border-weak',   'var(--border-weak)'],
            ['--border-medium', 'var(--border-medium)'],
            ['--border-strong', 'var(--border-strong)'],
          ].map(([token, val]) => (
            <div key={token} style={{
              padding: 'var(--space-3)',
              border: `1px solid ${val}`,
              borderRadius: 'var(--radius-md)',
              marginBottom: 'var(--space-2)',
              fontSize: 'var(--text-xs)',
              color: 'var(--text-secondary)',
              fontFamily: 'var(--font-mono)',
            }}>
              {token}
            </div>
          ))}
        </Card>

        {/* ── Text ────────────────────────────────────────────────────────── */}
        <Card title="Text">
          <div style={s.col}>
            <span style={{ color: 'var(--text-primary)',      fontSize: 'var(--text-md)' }}>--text-primary · Main body text</span>
            <span style={{ color: 'var(--text-secondary)',    fontSize: 'var(--text-md)' }}>--text-secondary · Labels and hints</span>
            <span style={{ color: 'var(--text-disabled)',     fontSize: 'var(--text-md)' }}>--text-disabled · Non-interactive</span>
            <span style={{ color: 'var(--text-link)',         fontSize: 'var(--text-md)' }}>--text-link · Anchor text</span>
            <span style={{ color: 'var(--text-max-contrast)', fontSize: 'var(--text-md)', background: 'var(--bg-secondary)', padding: '2px var(--space-2)', borderRadius: 'var(--radius-sm)' }}>--text-max-contrast</span>
          </div>
        </Card>

        {/* ── Type scale ──────────────────────────────────────────────────── */}
        <Card title="Type scale">
          <div style={s.col}>
            {[
              ['--text-4xl', '2rem',      'var(--weight-bold)',    'Heading 4xl'],
              ['--text-3xl', '1.5rem',    'var(--weight-bold)',    'Heading 3xl'],
              ['--text-2xl', '1.25rem',   'var(--weight-medium)',  'Heading 2xl'],
              ['--text-xl',  '1.125rem',  'var(--weight-medium)',  'Heading xl'],
              ['--text-lg',  '1rem',      'var(--weight-regular)', 'Body large'],
              ['--text-md',  '0.875rem',  'var(--weight-regular)', 'Body base (14px)'],
              ['--text-sm',  '0.75rem',   'var(--weight-regular)', 'Small'],
              ['--text-xs',  '0.6875rem', 'var(--weight-regular)', 'Extra small'],
            ].map(([token, size, weight, label]) => (
              <div key={token} style={{ display: 'flex', alignItems: 'baseline', gap: 'var(--space-3)' }}>
                <span style={{ fontSize: size, fontWeight: weight, color: 'var(--text-primary)', lineHeight: '1.4' }}>{label}</span>
                <span style={{ fontSize: 'var(--text-xs)', color: 'var(--text-disabled)', fontFamily: 'var(--font-mono)' }}>{token}</span>
              </div>
            ))}
          </div>
        </Card>

        {/* ── Buttons ─────────────────────────────────────────────────────── */}
        <Card title="Buttons">
          <div style={{ ...s.row, marginBottom: 'var(--space-3)' }}>
            <Button variant="primary">Primary</Button>
            <Button variant="secondary">Secondary</Button>
            <Button variant="success">Success</Button>
            <Button variant="error">Error</Button>
            <Button variant="warning">Warning</Button>
            <Button variant="ghost">Ghost</Button>
          </div>
          <div style={s.divider} />
          <div style={{ ...s.label, marginBottom: 'var(--space-2)' }}>Sizes</div>
          <div style={{ ...s.row, alignItems: 'flex-end' }}>
            <Button variant="primary" size="sm">Small</Button>
            <Button variant="primary" size="md">Medium</Button>
            <Button variant="primary" size="lg">Large</Button>
          </div>
          <div style={s.divider} />
          <div style={{ ...s.label, marginBottom: 'var(--space-2)' }}>Disabled</div>
          <div style={s.row}>
            <Button variant="primary" disabled>Primary</Button>
            <Button variant="secondary" disabled>Secondary</Button>
          </div>
        </Card>

        {/* ── Badges ──────────────────────────────────────────────────────── */}
        <Card title="Badges">
          <div style={s.row}>
            <Badge variant="primary">Info</Badge>
            <Badge variant="success">Running</Badge>
            <Badge variant="error">Failed</Badge>
            <Badge variant="warning">Degraded</Badge>
            <Badge variant="secondary">Inactive</Badge>
          </div>
        </Card>

        {/* ── Form inputs ─────────────────────────────────────────────────── */}
        <Card title="Form inputs">
          <div style={s.col}>
            <Input placeholder="Default input (click to focus)" />
            <Input placeholder="Disabled input" disabled />
          </div>
        </Card>

        {/* ── Semantic colors ──────────────────────────────────────────────── */}
        <Card title="Semantic colors">
          {(
            [
              ['Primary',   'var(--color-primary)',   'var(--color-primary-subtle)',   'var(--color-primary-text)',   'var(--color-primary-border)'],
              ['Success',   'var(--color-success)',   'var(--color-success-subtle)',   'var(--color-success-text)',   'var(--color-success-border)'],
              ['Error',     'var(--color-error)',     'var(--color-error-subtle)',     'var(--color-error-text)',     'var(--color-error-border)'],
              ['Warning',   'var(--color-warning)',   'var(--color-warning-subtle)',   'var(--color-warning-text)',   'var(--color-warning-border)'],
            ] as [string, string, string, string, string][]
          ).map(([name, main, subtle, text, border]) => (
            <div key={name} style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)', marginBottom: 'var(--space-2)' }}>
              <div style={{ width: 20, height: 20, borderRadius: 'var(--radius-sm)', background: main, flexShrink: 0 }} />
              <div style={{ width: 20, height: 20, borderRadius: 'var(--radius-sm)', background: subtle, border: `1px solid ${border}`, flexShrink: 0 }} />
              <span style={{ fontSize: 'var(--text-xs)', color: text, fontWeight: 'var(--weight-medium)', minWidth: 60 }}>{name}</span>
              <span style={{ fontSize: 'var(--text-xs)', color: 'var(--text-disabled)', fontFamily: 'var(--font-mono)' }}>main · subtle · text · border</span>
            </div>
          ))}
        </Card>

        {/* ── Shadows ─────────────────────────────────────────────────────── */}
        <Card title="Shadows">
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 'var(--space-3)' }}>
            <ShadowSwatch token="--shadow-xs" shadow="var(--shadow-xs)" />
            <ShadowSwatch token="--shadow-sm" shadow="var(--shadow-sm)" />
            <ShadowSwatch token="--shadow-md" shadow="var(--shadow-md)" />
            <ShadowSwatch token="--shadow-lg" shadow="var(--shadow-lg)" />
          </div>
        </Card>

        {/* ── Radius ──────────────────────────────────────────────────────── */}
        <Card title="Border radius">
          <div style={{ ...s.row, alignItems: 'flex-end' }}>
            {[
              ['--radius-sm', '3px'],
              ['--radius-md', '5px'],
              ['--radius-lg', '8px'],
              ['--radius-xl', '12px'],
              ['--radius-full', '9999px'],
            ].map(([token, val]) => (
              <div key={token} style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 'var(--space-2)' }}>
                <div style={{
                  width: 48,
                  height: 48,
                  background: 'var(--color-primary-subtle)',
                  border: '1px solid var(--color-primary-border)',
                  borderRadius: val,
                }} />
                <span style={{ fontSize: 'var(--text-xs)', color: 'var(--text-secondary)', fontFamily: 'var(--font-mono)', textAlign: 'center' }}>{val}</span>
              </div>
            ))}
          </div>
        </Card>

        {/* ── Spacing ─────────────────────────────────────────────────────── */}
        <Card title="Spacing scale">
          <div style={s.col}>
            {[
              ['--space-1',  '0.25rem'],
              ['--space-2',  '0.5rem'],
              ['--space-3',  '0.75rem'],
              ['--space-4',  '1rem'],
              ['--space-6',  '1.5rem'],
              ['--space-8',  '2rem'],
              ['--space-10', '2.5rem'],
              ['--space-12', '3rem'],
            ].map(([token, size]) => (
              <SpacingSwatch key={token} token={token} size={size} />
            ))}
          </div>
        </Card>

        {/* ── Monospace ───────────────────────────────────────────────────── */}
        <Card title="Monospace (--font-mono)">
          <pre style={{
            background: 'var(--bg-secondary)',
            border: '1px solid var(--border-weak)',
            borderRadius: 'var(--radius-md)',
            padding: 'var(--space-4)',
            fontSize: 'var(--text-sm)',
            color: 'var(--text-primary)',
            overflowX: 'auto',
            lineHeight: 'var(--leading-relaxed)',
          }}>{`--bg-canvas:    #1a2236
--bg-primary:   #212a44
--bg-secondary: #28324f
--bg-elevated:  #303c5e

--color-primary: #3d71d9
--color-success: #1a7f4b
--color-error:   #d10e5c
--color-warning: #ff9900`}</pre>
        </Card>

        {/* ── Elevated surface (dropdown-like) ────────────────────────────── */}
        <Card title="Elevated surface">
          <div style={{
            background: 'var(--bg-elevated)',
            border: '1px solid var(--border-medium)',
            borderRadius: 'var(--radius-lg)',
            boxShadow: 'var(--shadow-md)',
            overflow: 'hidden',
          }}>
            {['Dashboard', 'Profiles', 'Alerts', 'Settings'].map((item, i) => (
              <div
                key={item}
                style={{
                  padding: 'var(--space-2) var(--space-4)',
                  fontSize: 'var(--text-md)',
                  color: i === 1 ? 'var(--color-primary-text)' : 'var(--text-primary)',
                  background: i === 1 ? 'var(--action-selected)' : 'transparent',
                  borderBottom: i < 3 ? '1px solid var(--border-weak)' : 'none',
                  cursor: 'pointer',
                }}
              >
                {item}
              </div>
            ))}
          </div>
        </Card>

        {/* ── Motion tokens ───────────────────────────────────────────────── */}
        <Card title="Motion">
          <div style={s.col}>
            {[
              ['--duration-fast',    '100ms'],
              ['--duration-base',    '150ms'],
              ['--duration-slow',    '200ms'],
              ['--duration-slower',  '300ms'],
            ].map(([token, val]) => (
              <div key={token} style={{ display: 'flex', justifyContent: 'space-between', fontSize: 'var(--text-xs)', fontFamily: 'var(--font-mono)' }}>
                <span style={{ color: 'var(--text-primary)' }}>{token}</span>
                <span style={{ color: 'var(--text-disabled)' }}>{val}</span>
              </div>
            ))}
            <div style={s.divider} />
            {[
              ['--ease-smooth',  'cubic-bezier(0.4, 0, 0.2, 1)'],
              ['--ease-spring',  'cubic-bezier(0.34, 1.56, 0.64, 1)'],
            ].map(([token, val]) => (
              <div key={token} style={{ fontSize: 'var(--text-xs)', fontFamily: 'var(--font-mono)' }}>
                <div style={{ color: 'var(--text-primary)' }}>{token}</div>
                <div style={{ color: 'var(--text-disabled)' }}>{val}</div>
              </div>
            ))}
          </div>
        </Card>

        {/* ── Z-index ─────────────────────────────────────────────────────── */}
        <Card title="Z-index scale">
          <div style={s.col}>
            {[
              ['--z-raised',   '10'],
              ['--z-dropdown', '1000'],
              ['--z-sticky',   '1100'],
              ['--z-overlay',  '1200'],
              ['--z-modal',    '1300'],
              ['--z-popover',  '1400'],
              ['--z-toast',    '1500'],
              ['--z-tooltip',  '1600'],
            ].map(([token, val]) => (
              <div key={token} style={{ display: 'flex', justifyContent: 'space-between', fontSize: 'var(--text-xs)', fontFamily: 'var(--font-mono)' }}>
                <span style={{ color: 'var(--text-primary)' }}>{token}</span>
                <span style={{ color: 'var(--text-disabled)' }}>{val}</span>
              </div>
            ))}
          </div>
        </Card>

      </div>
    </div>
  )
}
