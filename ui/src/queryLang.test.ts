import { describe, it } from 'node:test';
import { strict as assert } from 'node:assert';

import {
  tokenize,
  getCursorContext,
  applySuggestion,
  buildQuery,
  parseQuery,
  toDisplayLabel,
  toInternalLabel,
  isInternalLabel,
} from './queryLang.ts';

describe('tokenize', () => {
  it('reports -1 for missing braces', () => {
    const t = tokenize('foo');
    assert.equal(t.braceOpen, -1);
    assert.equal(t.braceClose, -1);
    assert.deepEqual(t.matchers, []);
  });

  it('parses a single complete matcher', () => {
    const t = tokenize('{service_name="api"}');
    assert.equal(t.braceOpen, 0);
    assert.equal(t.braceClose, 19);
    assert.equal(t.matchers.length, 1);
    const [m] = t.matchers;
    assert.equal(m.name, 'service_name');
    assert.equal(m.op, '=');
    assert.equal(m.value, 'api');
    assert.equal(m.complete, true);
    assert.equal(m.valueClosed, true);
  });

  it('parses multiple matchers separated by commas', () => {
    const t = tokenize('{a="b",c="d"}');
    assert.equal(t.matchers.length, 2);
    assert.deepEqual(
      t.matchers.map((m) => m.name),
      ['a', 'c'],
    );
    assert.equal(
      t.matchers.every((m) => m.complete),
      true,
    );
  });

  it('does not split on commas inside a quoted value', () => {
    const t = tokenize('{labels="foo,bar"}');
    assert.equal(t.matchers.length, 1);
    assert.equal(t.matchers[0].value, 'foo,bar');
    assert.equal(t.matchers[0].complete, true);
  });

  it('handles escaped quotes inside values', () => {
    const t = tokenize('{a="foo\\"bar"}');
    assert.equal(t.matchers.length, 1);
    assert.equal(t.matchers[0].valueClosed, true);
    assert.equal(t.matchers[0].complete, true);
  });

  it('handles an unterminated quoted value', () => {
    const t = tokenize('{a="api');
    assert.equal(t.matchers.length, 1);
    assert.equal(t.matchers[0].valueClosed, false);
    assert.equal(t.matchers[0].complete, false);
    assert.equal(t.matchers[0].value, 'api');
  });

  it('handles empty braces with a single empty slot', () => {
    const t = tokenize('{}');
    assert.equal(t.matchers.length, 1);
    assert.equal(t.matchers[0].name, '');
    assert.equal(t.matchers[0].complete, false);
  });

  it('handles a partial name with no operator', () => {
    const t = tokenize('{serv');
    assert.equal(t.matchers.length, 1);
    assert.equal(t.matchers[0].name, 'serv');
    assert.equal(t.matchers[0].op, '');
    assert.equal(t.matchers[0].complete, false);
  });

  it('recognizes the four operators', () => {
    assert.equal(tokenize('{a="b"}').matchers[0].op, '=');
    assert.equal(tokenize('{a!="b"}').matchers[0].op, '!=');
    assert.equal(tokenize('{a=~"b"}').matchers[0].op, '=~');
    assert.equal(tokenize('{a!~"b"}').matchers[0].op, '!~');
  });

  it('preserves a trailing empty slot after a comma', () => {
    const t = tokenize('{a="b", }');
    assert.equal(t.matchers.length, 2);
    assert.equal(t.matchers[0].name, 'a');
    assert.equal(t.matchers[1].name, '');
  });
});

describe('getCursorContext', () => {
  it('returns none before the opening brace', () => {
    assert.equal(getCursorContext('{a="b"}', 0).kind, 'none');
    assert.equal(getCursorContext('foo{a="b"}', 2).kind, 'none');
  });

  it('returns none after the closing brace', () => {
    const q = '{a="b"}';
    assert.equal(getCursorContext(q, 7).kind, 'none');
  });

  it('suggests names right after the opening brace', () => {
    const ctx = getCursorContext('{}', 1);
    assert.equal(ctx.kind, 'name');
    if (ctx.kind !== 'name') return;
    assert.equal(ctx.prefix, '');
    assert.deepEqual(ctx.replaceRange, [1, 1]);
    assert.deepEqual(ctx.otherMatchers, []);
  });

  it('suggests names with a partial prefix', () => {
    const ctx = getCursorContext('{serv', 5);
    assert.equal(ctx.kind, 'name');
    if (ctx.kind !== 'name') return;
    assert.equal(ctx.prefix, 'serv');
    assert.deepEqual(ctx.replaceRange, [1, 5]);
  });

  it('suggests names with cursor inside a partial name', () => {
    const q = '{service}';
    const ctx = getCursorContext(q, 4);
    assert.equal(ctx.kind, 'name');
    if (ctx.kind !== 'name') return;
    assert.equal(ctx.prefix, 'ser');
  });

  it('suggests values with cursor inside a quoted value', () => {
    const q = '{a="api"}';
    const ctx = getCursorContext(q, 6);
    assert.equal(ctx.kind, 'value');
    if (ctx.kind !== 'value') return;
    assert.equal(ctx.labelName, 'a');
    assert.equal(ctx.prefix, 'ap');
    assert.deepEqual(ctx.replaceRange, [4, 7]);
  });

  it('suggests values with an empty value', () => {
    const q = '{a=""}';
    const ctx = getCursorContext(q, 4);
    assert.equal(ctx.kind, 'value');
    if (ctx.kind !== 'value') return;
    assert.equal(ctx.prefix, '');
    assert.deepEqual(ctx.replaceRange, [4, 4]);
  });

  it('suggests values for an unclosed quoted value', () => {
    const q = '{a="ap';
    const ctx = getCursorContext(q, 6);
    assert.equal(ctx.kind, 'value');
    if (ctx.kind !== 'value') return;
    assert.equal(ctx.prefix, 'ap');
    assert.deepEqual(ctx.replaceRange, [4, 6]);
  });

  it('returns none with cursor between operator and opening quote', () => {
    const q = '{a="b"}';
    const ctx = getCursorContext(q, 3);
    assert.equal(ctx.kind, 'none');
  });

  it('returns none with cursor past the closing quote', () => {
    const q = '{a="b"}';
    const ctx = getCursorContext(q, 6);
    assert.equal(ctx.kind, 'none');
  });

  it('includes other complete matchers in otherMatchers', () => {
    const q = '{region="us-east", service_name="api"}';
    const cursor = q.indexOf('api') + 2;
    const ctx = getCursorContext(q, cursor);
    assert.equal(ctx.kind, 'value');
    if (ctx.kind !== 'value') return;
    assert.deepEqual(ctx.otherMatchers, ['{region="us-east"}']);
  });

  it('excludes the matcher under the cursor from otherMatchers', () => {
    const q = '{a="x", b="y"}';
    const ctx = getCursorContext(q, q.indexOf('"y"') + 2);
    assert.equal(ctx.kind, 'value');
    if (ctx.kind !== 'value') return;
    assert.deepEqual(ctx.otherMatchers, ['{a="x"}']);
  });

  it('suggests names in a new slot after a comma', () => {
    const q = '{a="b", }';
    const ctx = getCursorContext(q, 8);
    assert.equal(ctx.kind, 'name');
    if (ctx.kind !== 'name') return;
    assert.equal(ctx.prefix, '');
    assert.deepEqual(ctx.otherMatchers, ['{a="b"}']);
  });
});

describe('applySuggestion', () => {
  it('appends ="| when accepting a name with no following operator', () => {
    const q = '{}';
    const ctx = getCursorContext(q, 1);
    const { next, cursor } = applySuggestion(q, ctx, 'service_name');
    assert.equal(next, '{service_name="}');
    assert.equal(cursor, next.indexOf('"') + 1);
  });

  it('replaces a partial name and adds ="', () => {
    const q = '{serv}';
    const ctx = getCursorContext(q, 5);
    const { next, cursor } = applySuggestion(q, ctx, 'service_name');
    assert.equal(next, '{service_name="}');
    assert.equal(cursor, next.indexOf('"') + 1);
  });

  it('replaces a name without inserting an operator when one already follows', () => {
    const q = '{a="b"}';
    const ctx = getCursorContext(q, 2);
    const { next, cursor } = applySuggestion(q, ctx, 'region');
    assert.equal(next, '{region="b"}');
    assert.equal(cursor, '{region'.length);
  });

  it('replaces a partial value and leaves the closing quote intact', () => {
    const q = '{a="ap"}';
    const ctx = getCursorContext(q, q.indexOf('ap') + 2);
    const { next, cursor } = applySuggestion(q, ctx, 'api');
    assert.equal(next, '{a="api"}');
    assert.equal(cursor, next.indexOf('"', 4) + 1);
  });

  it('appends a closing quote when the value is unclosed', () => {
    const q = '{a="ap';
    const ctx = getCursorContext(q, q.length);
    const { next, cursor } = applySuggestion(q, ctx, 'api');
    assert.equal(next, '{a="api"');
    assert.equal(cursor, next.length);
  });

  it('handles accepting a value for an empty quoted string', () => {
    const q = '{a=""}';
    const ctx = getCursorContext(q, 4);
    const { next } = applySuggestion(q, ctx, 'api');
    assert.equal(next, '{a="api"}');
  });
});

describe('label name translation', () => {
  it('maps display names to their internal form', () => {
    assert.equal(toInternalLabel('profile_type'), '__profile_type__');
    assert.equal(toInternalLabel('service_name'), 'service_name');
  });

  it('maps internal names to their display form', () => {
    assert.equal(toDisplayLabel('__profile_type__'), 'profile_type');
    assert.equal(toDisplayLabel('service_name'), 'service_name');
  });

  it('detects internal __xxx__ labels', () => {
    assert.equal(isInternalLabel('__delta__'), true);
    assert.equal(isInternalLabel('__session_id__'), true);
    assert.equal(isInternalLabel('__profile_type__'), true);
    assert.equal(isInternalLabel('service_name'), false);
    assert.equal(isInternalLabel('profile_type'), false);
    assert.equal(isInternalLabel('__partial'), false);
    assert.equal(isInternalLabel('partial__'), false);
  });

  it('serializes profile_type matchers using the internal label name', () => {
    const q = '{profile_type="cpu", service_name="ap"}';
    const cursor = q.indexOf('"ap"') + 2;
    const ctx = getCursorContext(q, cursor);
    assert.equal(ctx.kind, 'value');
    if (ctx.kind !== 'value') return;
    assert.deepEqual(ctx.otherMatchers, ['{__profile_type__="cpu"}']);
  });
});

describe('parseQuery / buildQuery', () => {
  it('round-trips a built query', () => {
    const q = buildQuery(
      'my-service',
      'process_cpu:cpu:nanoseconds:cpu:nanoseconds',
    );
    const parsed = parseQuery(q);
    assert.deepEqual(parsed, {
      service: 'my-service',
      profileType: 'process_cpu:cpu:nanoseconds:cpu:nanoseconds',
    });
  });

  it('returns null when required labels are absent', () => {
    assert.equal(parseQuery('{foo="bar"}'), null);
  });

  it('tolerates extra whitespace around the operator', () => {
    const parsed = parseQuery('{service_name = "api", profile_type = "cpu"}');
    assert.deepEqual(parsed, { service: 'api', profileType: 'cpu' });
  });
});
