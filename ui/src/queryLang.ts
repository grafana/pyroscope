export type Operator = '=' | '!=' | '=~' | '!~';

export interface Matcher {
  name: string;
  op: Operator | '';
  value: string;
  start: number;
  end: number;
  nameStart: number;
  nameEnd: number;
  opStart: number;
  opEnd: number;
  valueStart: number;
  valueEnd: number;
  valueClosed: boolean;
  complete: boolean;
}

export interface Tokens {
  matchers: Matcher[];
  braceOpen: number;
  braceClose: number;
}

export type CursorContext =
  | {
      kind: 'name';
      prefix: string;
      replaceRange: [number, number];
      otherMatchers: string[];
    }
  | {
      kind: 'value';
      labelName: string;
      prefix: string;
      replaceRange: [number, number];
      otherMatchers: string[];
    }
  | { kind: 'none' };

const IDENT = /[A-Za-z0-9_]/;
const WS = /\s/;

// Label names that are presented to the user in a friendlier form than the
// backend uses. The query bar accepts and renders the display name; we
// translate to/from the internal name whenever a matcher crosses the API.
const DISPLAY_TO_INTERNAL: Record<string, string> = {
  profile_type: '__profile_type__',
};
const INTERNAL_TO_DISPLAY: Record<string, string> = Object.fromEntries(
  Object.entries(DISPLAY_TO_INTERNAL).map(([d, i]) => [i, d]),
);

export function toInternalLabel(name: string): string {
  return DISPLAY_TO_INTERNAL[name] ?? name;
}

export function toDisplayLabel(name: string): string {
  return INTERNAL_TO_DISPLAY[name] ?? name;
}

// Labels wrapped in double underscores (e.g. __delta__, __session_id__) are
// reserved internals and should not surface in autocomplete. Aliased labels
// like __profile_type__ are first translated via `toDisplayLabel`, so this
// check sees the display form (`profile_type`) and lets them through.
export function isInternalLabel(name: string): boolean {
  return name.startsWith('__') && name.endsWith('__');
}

export function tokenize(q: string): Tokens {
  const braceOpen = q.indexOf('{');
  if (braceOpen === -1) {
    return { matchers: [], braceOpen: -1, braceClose: -1 };
  }

  let braceClose = -1;
  let inQuote = false;
  let escaped = false;
  for (let i = braceOpen + 1; i < q.length; i++) {
    const ch = q[i];
    if (escaped) {
      escaped = false;
      continue;
    }
    if (inQuote) {
      if (ch === '\\') escaped = true;
      else if (ch === '"') inQuote = false;
      continue;
    }
    if (ch === '"') inQuote = true;
    else if (ch === '}') {
      braceClose = i;
      break;
    }
  }

  const contentEnd = braceClose === -1 ? q.length : braceClose;
  const slots: Array<[number, number]> = [];
  let slotStart = braceOpen + 1;
  inQuote = false;
  escaped = false;
  for (let i = braceOpen + 1; i < contentEnd; i++) {
    const ch = q[i];
    if (escaped) {
      escaped = false;
      continue;
    }
    if (inQuote) {
      if (ch === '\\') escaped = true;
      else if (ch === '"') inQuote = false;
      continue;
    }
    if (ch === '"') inQuote = true;
    else if (ch === ',') {
      slots.push([slotStart, i]);
      slotStart = i + 1;
    }
  }
  slots.push([slotStart, contentEnd]);

  const matchers = slots.map(([s, e]) => parseSlot(q, s, e));
  return { matchers, braceOpen, braceClose };
}

function parseSlot(q: string, slotStart: number, slotEnd: number): Matcher {
  let i = slotStart;
  while (i < slotEnd && WS.test(q[i])) i++;
  const nameStart = i;
  while (i < slotEnd && IDENT.test(q[i])) i++;
  const nameEnd = i;
  const name = q.slice(nameStart, nameEnd);

  while (i < slotEnd && WS.test(q[i])) i++;
  const opStart = i;
  let op: Operator | '' = '';
  const c0 = q[i];
  const c1 = q[i + 1];
  if (c0 === '!' && c1 === '=') {
    op = '!=';
    i += 2;
  } else if (c0 === '!' && c1 === '~') {
    op = '!~';
    i += 2;
  } else if (c0 === '=' && c1 === '~') {
    op = '=~';
    i += 2;
  } else if (c0 === '=') {
    op = '=';
    i += 1;
  }
  const opEnd = i;

  while (i < slotEnd && WS.test(q[i])) i++;

  let valueStart = -1;
  let valueEnd = -1;
  let value = '';
  let valueClosed = false;
  if (q[i] === '"') {
    valueStart = i;
    i++;
    const textStart = i;
    let esc = false;
    while (i < slotEnd) {
      if (esc) {
        esc = false;
        i++;
        continue;
      }
      if (q[i] === '\\') {
        esc = true;
        i++;
        continue;
      }
      if (q[i] === '"') {
        valueEnd = i;
        valueClosed = true;
        i++;
        break;
      }
      i++;
    }
    if (!valueClosed) valueEnd = i;
    value = q.slice(textStart, valueEnd);
  }

  const complete = name.length > 0 && op !== '' && valueClosed;

  return {
    name,
    op,
    value,
    start: slotStart,
    end: slotEnd,
    nameStart,
    nameEnd,
    opStart,
    opEnd,
    valueStart,
    valueEnd,
    valueClosed,
    complete,
  };
}

function serializeMatcher(m: Matcher): string {
  return `{${toInternalLabel(m.name)}${m.op}"${m.value}"}`;
}

export function getCursorContext(
  query: string,
  cursor: number,
): CursorContext {
  const { matchers, braceOpen, braceClose } = tokenize(query);
  if (braceOpen === -1) return { kind: 'none' };
  if (cursor <= braceOpen) return { kind: 'none' };
  if (braceClose !== -1 && cursor > braceClose) return { kind: 'none' };

  const slot = matchers.find((m) => cursor >= m.start && cursor <= m.end);
  if (!slot) return { kind: 'none' };

  const otherMatchers = matchers
    .filter((m) => m !== slot && m.complete)
    .map(serializeMatcher);

  // Cursor inside a quoted value (between the quotes).
  if (slot.valueStart !== -1 && cursor > slot.valueStart) {
    const closingPos = slot.valueClosed ? slot.valueEnd : -1;
    const inValue =
      closingPos === -1 ? cursor <= slot.end : cursor <= closingPos;
    if (inValue) {
      const prefix = query.slice(slot.valueStart + 1, cursor);
      const replaceEnd = closingPos === -1 ? cursor : closingPos;
      return {
        kind: 'value',
        labelName: slot.name,
        prefix,
        replaceRange: [slot.valueStart + 1, replaceEnd],
        otherMatchers,
      };
    }
  }

  // Cursor in or right after the label-name token (no op typed yet, or op being typed).
  // We treat positions from slot start up to (and including) nameEnd as "name" position,
  // as long as no operator has been parsed.
  if (slot.op === '' && cursor <= slot.nameEnd + 0) {
    const prefix = query.slice(slot.nameStart, cursor);
    return {
      kind: 'name',
      prefix,
      replaceRange: [slot.nameStart, Math.max(slot.nameEnd, cursor)],
      otherMatchers,
    };
  }

  // Cursor strictly inside the name (when op is also typed) — still complete the name.
  if (cursor >= slot.nameStart && cursor <= slot.nameEnd) {
    const prefix = query.slice(slot.nameStart, cursor);
    return {
      kind: 'name',
      prefix,
      replaceRange: [slot.nameStart, slot.nameEnd],
      otherMatchers,
    };
  }

  return { kind: 'none' };
}

export function applySuggestion(
  query: string,
  context: CursorContext,
  suggestion: string,
): { next: string; cursor: number } {
  if (context.kind === 'none') return { next: query, cursor: query.length };

  if (context.kind === 'name') {
    const [s, e] = context.replaceRange;
    const after = query[e] ?? '';
    const needsOp = after !== '=' && after !== '!';
    const insertion = needsOp ? `${suggestion}="` : suggestion;
    const next = query.slice(0, s) + insertion + query.slice(e);
    const cursor = s + insertion.length;
    return { next, cursor };
  }

  // value
  const [s, e] = context.replaceRange;
  const after = query[e] ?? '';
  const needsCloseQuote = after !== '"';
  const insertion = needsCloseQuote ? `${suggestion}"` : suggestion;
  const next = query.slice(0, s) + insertion + query.slice(e);
  // Cursor lands after the closing quote.
  const cursor = needsCloseQuote ? s + insertion.length : s + suggestion.length + 1;
  return { next, cursor };
}

export function buildQuery(service: string, profileType: string): string {
  return `{service_name="${service}", profile_type="${profileType}"}`;
}

export function parseQuery(
  q: string,
): { service: string; profileType: string } | null {
  const service = q.match(/service_name\s*=\s*"([^"]+)"/)?.[1];
  const profileType = q.match(/profile_type\s*=\s*"([^"]+)"/)?.[1];
  if (!service || !profileType) return null;
  return { service, profileType };
}
