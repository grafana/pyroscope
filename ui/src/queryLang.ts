export type Operator = '=' | '!=' | '=~' | '!~';

export interface Matcher {
  name: string;
  op: Operator | '';
  value: string;
  start: number;
  end: number;
  nameStart: number;
  nameEnd: number;
  // Range of the name's characters, excluding the quotes of a quoted name.
  nameTextStart: number;
  nameTextEnd: number;
  // False only for a quoted name that is missing its closing quote.
  nameClosed: boolean;
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
const LEGACY_LABEL_NAME = /^[a-zA-Z_][a-zA-Z0-9_]*$/;

// Label names outside the legacy charset (e.g. OTel's service.namespace) must
// be quoted in selectors: {"service.namespace"="prod"}. JSON string escapes
// are a subset of the Go escapes the backend's Prometheus parser accepts.
export function formatLabelName(name: string): string {
  return LEGACY_LABEL_NAME.test(name) ? name : JSON.stringify(name);
}

function unquoteLabelName(text: string): string {
  try {
    return JSON.parse(`"${text}"`) as string;
  } catch {
    return text;
  }
}

// Labels wrapped in double underscores (e.g. __delta__, __session_id__) are
// reserved internals and should not surface in autocomplete.
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

// Scans the double-quoted string whose opening quote is at q[start]. Returns
// the index of the closing quote, or limit if the string is unterminated.
function scanQuoted(
  q: string,
  start: number,
  limit: number,
): { close: number; closed: boolean } {
  let esc = false;
  for (let i = start + 1; i < limit; i++) {
    if (esc) esc = false;
    else if (q[i] === '\\') esc = true;
    else if (q[i] === '"') return { close: i, closed: true };
  }
  return { close: limit, closed: false };
}

function parseSlot(q: string, slotStart: number, slotEnd: number): Matcher {
  let i = slotStart;
  while (i < slotEnd && WS.test(q[i])) i++;
  const nameStart = i;
  let nameTextStart = i;
  let nameTextEnd: number;
  let nameClosed = true;
  let name: string;
  if (q[i] === '"') {
    const { close, closed } = scanQuoted(q, i, slotEnd);
    nameTextStart = i + 1;
    nameTextEnd = close;
    nameClosed = closed;
    name = unquoteLabelName(q.slice(nameTextStart, nameTextEnd));
    i = closed ? close + 1 : close;
  } else {
    while (i < slotEnd && IDENT.test(q[i])) i++;
    nameTextEnd = i;
    name = q.slice(nameStart, nameTextEnd);
  }
  const nameEnd = i;

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
    const { close, closed } = scanQuoted(q, i, slotEnd);
    valueEnd = close;
    valueClosed = closed;
    value = q.slice(valueStart + 1, valueEnd);
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
    nameTextStart,
    nameTextEnd,
    nameClosed,
    opStart,
    opEnd,
    valueStart,
    valueEnd,
    valueClosed,
    complete,
  };
}

function serializeMatcher(m: Matcher): string {
  return `{${formatLabelName(m.name)}${m.op}"${m.value}"}`;
}

export function getCursorContext(query: string, cursor: number): CursorContext {
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

  const namePrefix = query.slice(
    slot.nameTextStart,
    Math.min(cursor, slot.nameTextEnd),
  );

  // Cursor in or right after the label-name token (no op typed yet, or op being typed).
  // We treat positions from slot start up to (and including) nameEnd as "name" position,
  // as long as no operator has been parsed.
  // An unterminated quoted name runs to the end of the slot, so like an
  // unclosed value it is only replaced up to the cursor.
  if (slot.op === '' && cursor <= slot.nameEnd) {
    const replaceEnd = slot.nameClosed
      ? Math.max(slot.nameEnd, cursor)
      : cursor;
    return {
      kind: 'name',
      prefix: namePrefix,
      replaceRange: [slot.nameStart, replaceEnd],
      otherMatchers,
    };
  }

  // Cursor strictly inside the name (when op is also typed) — still complete the name.
  if (cursor >= slot.nameStart && cursor <= slot.nameEnd) {
    return {
      kind: 'name',
      prefix: namePrefix,
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
    const name = formatLabelName(suggestion);
    const insertion = needsOp ? `${name}="` : name;
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
  const cursor = needsCloseQuote
    ? s + insertion.length
    : s + suggestion.length + 1;
  return { next, cursor };
}

export function buildQuery(service: string): string {
  return `{service_name="${service}"}`;
}

export function parseQuery(
  q: string,
): { service: string; labelSelector: string } | null {
  const { matchers, braceOpen } = tokenize(q);
  if (braceOpen === -1) return null;

  let service = '';
  for (const m of matchers) {
    if (!m.complete) continue;
    if (m.name === 'service_name' && m.op === '=') {
      service = m.value;
      break;
    }
  }

  if (!service) return null;

  return { service, labelSelector: q.trim() };
}
