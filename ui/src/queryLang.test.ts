import { describe, it, expect } from 'vitest';
import {
  tokenize,
  getCursorContext,
  applySuggestion,
  buildQuery,
  parseQuery,
  toDisplayLabel,
  toInternalLabel,
  isInternalLabel,
} from './queryLang';

describe('tokenize', () => {
  it('reports -1 for missing braces', () => {
    const t = tokenize('foo');
    expect(t.braceOpen).toBe(-1);
    expect(t.braceClose).toBe(-1);
    expect(t.matchers).toEqual([]);
  });

  it('parses a single complete matcher', () => {
    const t = tokenize('{service_name="api"}');
    expect(t.braceOpen).toBe(0);
    expect(t.braceClose).toBe(19);
    expect(t.matchers).toHaveLength(1);
    const [m] = t.matchers;
    expect(m.name).toBe('service_name');
    expect(m.op).toBe('=');
    expect(m.value).toBe('api');
    expect(m.complete).toBe(true);
    expect(m.valueClosed).toBe(true);
  });

  it('parses multiple matchers separated by commas', () => {
    const t = tokenize('{a="b",c="d"}');
    expect(t.matchers).toHaveLength(2);
    expect(t.matchers.map((m) => m.name)).toEqual(['a', 'c']);
    expect(t.matchers.every((m) => m.complete)).toBe(true);
  });

  it('does not split on commas inside a quoted value', () => {
    const t = tokenize('{labels="foo,bar"}');
    expect(t.matchers).toHaveLength(1);
    expect(t.matchers[0].value).toBe('foo,bar');
    expect(t.matchers[0].complete).toBe(true);
  });

  it('handles escaped quotes inside values', () => {
    const t = tokenize('{a="foo\\"bar"}');
    expect(t.matchers).toHaveLength(1);
    expect(t.matchers[0].valueClosed).toBe(true);
    expect(t.matchers[0].complete).toBe(true);
  });

  it('handles an unterminated quoted value', () => {
    const t = tokenize('{a="api');
    expect(t.matchers).toHaveLength(1);
    expect(t.matchers[0].valueClosed).toBe(false);
    expect(t.matchers[0].complete).toBe(false);
    expect(t.matchers[0].value).toBe('api');
  });

  it('handles empty braces with a single empty slot', () => {
    const t = tokenize('{}');
    expect(t.matchers).toHaveLength(1);
    expect(t.matchers[0].name).toBe('');
    expect(t.matchers[0].complete).toBe(false);
  });

  it('handles a partial name with no operator', () => {
    const t = tokenize('{serv');
    expect(t.matchers).toHaveLength(1);
    expect(t.matchers[0].name).toBe('serv');
    expect(t.matchers[0].op).toBe('');
    expect(t.matchers[0].complete).toBe(false);
  });

  it('recognizes the four operators', () => {
    expect(tokenize('{a="b"}').matchers[0].op).toBe('=');
    expect(tokenize('{a!="b"}').matchers[0].op).toBe('!=');
    expect(tokenize('{a=~"b"}').matchers[0].op).toBe('=~');
    expect(tokenize('{a!~"b"}').matchers[0].op).toBe('!~');
  });

  it('preserves a trailing empty slot after a comma', () => {
    const t = tokenize('{a="b", }');
    expect(t.matchers).toHaveLength(2);
    expect(t.matchers[0].name).toBe('a');
    expect(t.matchers[1].name).toBe('');
  });
});

describe('getCursorContext', () => {
  it('returns none before the opening brace', () => {
    expect(getCursorContext('{a="b"}', 0).kind).toBe('none');
    expect(getCursorContext('foo{a="b"}', 2).kind).toBe('none');
  });

  it('returns none after the closing brace', () => {
    const q = '{a="b"}';
    // braceClose is at index 6
    expect(getCursorContext(q, 7).kind).toBe('none');
  });

  it('suggests names right after the opening brace', () => {
    const ctx = getCursorContext('{}', 1);
    expect(ctx.kind).toBe('name');
    if (ctx.kind !== 'name') return;
    expect(ctx.prefix).toBe('');
    expect(ctx.replaceRange).toEqual([1, 1]);
    expect(ctx.otherMatchers).toEqual([]);
  });

  it('suggests names with a partial prefix', () => {
    const ctx = getCursorContext('{serv', 5);
    expect(ctx.kind).toBe('name');
    if (ctx.kind !== 'name') return;
    expect(ctx.prefix).toBe('serv');
    expect(ctx.replaceRange).toEqual([1, 5]);
  });

  it('suggests names with cursor inside a partial name', () => {
    // {ser|vice}
    const q = '{service}';
    const ctx = getCursorContext(q, 4);
    expect(ctx.kind).toBe('name');
    if (ctx.kind !== 'name') return;
    expect(ctx.prefix).toBe('ser');
  });

  it('suggests values with cursor inside a quoted value', () => {
    // {a="ap|i"}
    const q = '{a="api"}';
    const ctx = getCursorContext(q, 6);
    expect(ctx.kind).toBe('value');
    if (ctx.kind !== 'value') return;
    expect(ctx.labelName).toBe('a');
    expect(ctx.prefix).toBe('ap');
    expect(ctx.replaceRange).toEqual([4, 7]);
  });

  it('suggests values with an empty value', () => {
    // {a="|"}
    const q = '{a=""}';
    const ctx = getCursorContext(q, 4);
    expect(ctx.kind).toBe('value');
    if (ctx.kind !== 'value') return;
    expect(ctx.prefix).toBe('');
    expect(ctx.replaceRange).toEqual([4, 4]);
  });

  it('suggests values for an unclosed quoted value', () => {
    // {a="ap|
    const q = '{a="ap';
    const ctx = getCursorContext(q, 6);
    expect(ctx.kind).toBe('value');
    if (ctx.kind !== 'value') return;
    expect(ctx.prefix).toBe('ap');
    expect(ctx.replaceRange).toEqual([4, 6]);
  });

  it('returns none with cursor between operator and opening quote', () => {
    // {a=|"b"}
    const q = '{a="b"}';
    const ctx = getCursorContext(q, 3);
    expect(ctx.kind).toBe('none');
  });

  it('returns none with cursor past the closing quote', () => {
    // {a="b"|}
    const q = '{a="b"}';
    const ctx = getCursorContext(q, 6);
    expect(ctx.kind).toBe('none');
  });

  it('includes other complete matchers in otherMatchers', () => {
    // {region="us-east", service_name="ap|"}
    const q = '{region="us-east", service_name="api"}';
    // Find cursor inside "api"
    const cursor = q.indexOf('api') + 2; // after 'ap'
    const ctx = getCursorContext(q, cursor);
    expect(ctx.kind).toBe('value');
    if (ctx.kind !== 'value') return;
    expect(ctx.otherMatchers).toEqual(['{region="us-east"}']);
  });

  it('excludes the matcher under the cursor from otherMatchers', () => {
    const q = '{a="x", b="y"}';
    // Cursor inside "y"
    const ctx = getCursorContext(q, q.indexOf('"y"') + 2);
    expect(ctx.kind).toBe('value');
    if (ctx.kind !== 'value') return;
    expect(ctx.otherMatchers).toEqual(['{a="x"}']);
  });

  it('suggests names in a new slot after a comma', () => {
    const q = '{a="b", }';
    // Cursor right before the closing brace
    const ctx = getCursorContext(q, 8);
    expect(ctx.kind).toBe('name');
    if (ctx.kind !== 'name') return;
    expect(ctx.prefix).toBe('');
    expect(ctx.otherMatchers).toEqual(['{a="b"}']);
  });
});

describe('applySuggestion', () => {
  it('appends ="| when accepting a name with no following operator', () => {
    const q = '{}';
    const ctx = getCursorContext(q, 1);
    const { next, cursor } = applySuggestion(q, ctx, 'service_name');
    expect(next).toBe('{service_name="}');
    // cursor between the quotes
    expect(cursor).toBe(next.indexOf('"') + 1);
  });

  it('replaces a partial name and adds ="', () => {
    const q = '{serv}';
    const ctx = getCursorContext(q, 5);
    const { next, cursor } = applySuggestion(q, ctx, 'service_name');
    expect(next).toBe('{service_name="}');
    expect(cursor).toBe(next.indexOf('"') + 1);
  });

  it('replaces a name without inserting an operator when one already follows', () => {
    const q = '{a="b"}';
    // Cursor right after 'a'
    const ctx = getCursorContext(q, 2);
    const { next, cursor } = applySuggestion(q, ctx, 'region');
    expect(next).toBe('{region="b"}');
    // cursor lands right after 'region', before '='
    expect(cursor).toBe('{region'.length);
  });

  it('replaces a partial value and leaves the closing quote intact', () => {
    const q = '{a="ap"}';
    const ctx = getCursorContext(q, q.indexOf('ap') + 2);
    const { next, cursor } = applySuggestion(q, ctx, 'api');
    expect(next).toBe('{a="api"}');
    // cursor lands right after the closing quote
    expect(cursor).toBe(next.indexOf('"', 4) + 1);
  });

  it('appends a closing quote when the value is unclosed', () => {
    const q = '{a="ap';
    const ctx = getCursorContext(q, q.length);
    const { next, cursor } = applySuggestion(q, ctx, 'api');
    expect(next).toBe('{a="api"');
    expect(cursor).toBe(next.length);
  });

  it('handles accepting a value for an empty quoted string', () => {
    const q = '{a=""}';
    const ctx = getCursorContext(q, 4);
    const { next } = applySuggestion(q, ctx, 'api');
    expect(next).toBe('{a="api"}');
  });
});

describe('label name translation', () => {
  it('maps display names to their internal form', () => {
    expect(toInternalLabel('profile_type')).toBe('__profile_type__');
    expect(toInternalLabel('service_name')).toBe('service_name');
  });

  it('maps internal names to their display form', () => {
    expect(toDisplayLabel('__profile_type__')).toBe('profile_type');
    expect(toDisplayLabel('service_name')).toBe('service_name');
  });

  it('detects internal __xxx__ labels', () => {
    expect(isInternalLabel('__delta__')).toBe(true);
    expect(isInternalLabel('__session_id__')).toBe(true);
    expect(isInternalLabel('__profile_type__')).toBe(true);
    expect(isInternalLabel('service_name')).toBe(false);
    expect(isInternalLabel('profile_type')).toBe(false);
    expect(isInternalLabel('__partial')).toBe(false);
    expect(isInternalLabel('partial__')).toBe(false);
  });

  it('serializes profile_type matchers using the internal label name', () => {
    const q = '{profile_type="cpu", service_name="ap"}';
    const cursor = q.indexOf('"ap"') + 2;
    const ctx = getCursorContext(q, cursor);
    expect(ctx.kind).toBe('value');
    if (ctx.kind !== 'value') return;
    expect(ctx.otherMatchers).toEqual(['{__profile_type__="cpu"}']);
  });
});

describe('parseQuery / buildQuery', () => {
  it('round-trips a built query', () => {
    const q = buildQuery(
      'my-service',
      'process_cpu:cpu:nanoseconds:cpu:nanoseconds',
    );
    const parsed = parseQuery(q);
    expect(parsed).toEqual({
      service: 'my-service',
      profileType: 'process_cpu:cpu:nanoseconds:cpu:nanoseconds',
    });
  });

  it('returns null when required labels are absent', () => {
    expect(parseQuery('{foo="bar"}')).toBeNull();
  });

  it('tolerates extra whitespace around the operator', () => {
    const parsed = parseQuery('{service_name = "api", profile_type = "cpu"}');
    expect(parsed).toEqual({ service: 'api', profileType: 'cpu' });
  });
});
