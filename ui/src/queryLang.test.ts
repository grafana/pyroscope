import { describe, it } from 'node:test';
import { strict as assert } from 'node:assert';

import {
  tokenize,
  getCursorContext,
  applySuggestion,
  buildQuery,
  parseQuery,
  isInternalLabel,
  formatLabelName,
} from './queryLang.ts';

describe('formatLabelName', () => {
  it('quotes only non-legacy label names', () => {
    assert.equal(formatLabelName('service_name'), 'service_name');
    assert.equal(formatLabelName('_private'), '_private');
    assert.equal(formatLabelName('Region9'), 'Region9');
    assert.equal(formatLabelName('service.name'), '"service.name"');
    assert.equal(formatLabelName('k8s-pod'), '"k8s-pod"');
    assert.equal(formatLabelName('9lives'), '"9lives"');
    assert.equal(formatLabelName('名前'), '"名前"');
  });

  it('escapes quotes and backslashes inside quoted names', () => {
    assert.equal(formatLabelName('a"b\\c'), '"a\\"b\\\\c"');
  });
});

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

  it('parses a quoted label name', () => {
    const t = tokenize('{"service.name"="api"}');
    assert.equal(t.matchers.length, 1);
    const [m] = t.matchers;
    assert.equal(m.name, 'service.name');
    assert.equal(m.op, '=');
    assert.equal(m.value, 'api');
    assert.equal(m.complete, true);
  });

  it('parses quoted label names alongside unquoted ones', () => {
    const t = tokenize('{"k8s.pod,name" != "x", env=~"prod"}');
    assert.deepEqual(
      t.matchers.map((m) => [m.name, m.op, m.value, m.complete]),
      [
        ['k8s.pod,name', '!=', 'x', true],
        ['env', '=~', 'prod', true],
      ],
    );
  });

  it('unescapes quoted label names', () => {
    const t = tokenize('{"a\\"b"="x"}');
    assert.equal(t.matchers[0].name, 'a"b');
    assert.equal(t.matchers[0].complete, true);
  });

  it('handles an unterminated quoted label name', () => {
    const t = tokenize('{"service.na');
    assert.equal(t.matchers.length, 1);
    assert.equal(t.matchers[0].name, 'service.na');
    assert.equal(t.matchers[0].op, '');
    assert.equal(t.matchers[0].complete, false);
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

  it('suggests values for a quoted label name', () => {
    const q = '{"service.name"="ap"}';
    const ctx = getCursorContext(q, q.indexOf('ap') + 2);
    assert.equal(ctx.kind, 'value');
    if (ctx.kind !== 'value') return;
    assert.equal(ctx.labelName, 'service.name');
    assert.equal(ctx.prefix, 'ap');
    assert.deepEqual(ctx.replaceRange, [17, 19]);
  });

  it('suggests names inside an unterminated quoted name', () => {
    const q = '{"service.na';
    const ctx = getCursorContext(q, q.length);
    assert.equal(ctx.kind, 'name');
    if (ctx.kind !== 'name') return;
    assert.equal(ctx.prefix, 'service.na');
    assert.deepEqual(ctx.replaceRange, [1, q.length]);
  });

  it('excludes the quotes from the prefix of a quoted name', () => {
    const q = '{"service.name"}';
    const ctx = getCursorContext(q, q.indexOf('}'));
    assert.equal(ctx.kind, 'name');
    if (ctx.kind !== 'name') return;
    assert.equal(ctx.prefix, 'service.name');
    assert.deepEqual(ctx.replaceRange, [1, q.indexOf('}')]);
  });

  it('quotes non-legacy names in otherMatchers', () => {
    const q = '{"service.name"="api", "region"="us", env=""}';
    const ctx = getCursorContext(q, q.lastIndexOf('"'));
    assert.equal(ctx.kind, 'value');
    if (ctx.kind !== 'value') return;
    assert.deepEqual(ctx.otherMatchers, [
      '{"service.name"="api"}',
      '{region="us"}',
    ]);
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

  it('quotes an accepted non-legacy name', () => {
    const q = '{serv}';
    const ctx = getCursorContext(q, 5);
    const { next, cursor } = applySuggestion(q, ctx, 'service.name');
    assert.equal(next, '{"service.name"="}');
    assert.equal(cursor, '{"service.name"="'.length);
  });

  it('replaces a quoted name including its quotes', () => {
    const q = '{"a.b"="x"}';
    const ctx = getCursorContext(q, 3);
    const { next, cursor } = applySuggestion(q, ctx, 'region');
    assert.equal(next, '{region="x"}');
    assert.equal(cursor, '{region'.length);
  });

  it('replaces an unterminated quoted name', () => {
    const q = '{"service.na';
    const ctx = getCursorContext(q, q.length);
    const { next } = applySuggestion(q, ctx, 'service.name');
    assert.equal(next, '{"service.name"="');
  });

  it('keeps text after the cursor in an unterminated quoted name', () => {
    const q = '{env="prod", "serv}';
    const ctx = getCursorContext(q, q.indexOf('}'));
    const { next } = applySuggestion(q, ctx, 'service.name');
    assert.equal(next, '{env="prod", "service.name"="}');
  });

  it('handles accepting a value for an empty quoted string', () => {
    const q = '{a=""}';
    const ctx = getCursorContext(q, 4);
    const { next } = applySuggestion(q, ctx, 'api');
    assert.equal(next, '{a="api"}');
  });
});

describe('label name translation', () => {
  it('detects internal __xxx__ labels', () => {
    assert.equal(isInternalLabel('__delta__'), true);
    assert.equal(isInternalLabel('__session_id__'), true);
    assert.equal(isInternalLabel('service_name'), false);
    assert.equal(isInternalLabel('__partial'), false);
    assert.equal(isInternalLabel('partial__'), false);
  });
});

describe('parseQuery / buildQuery', () => {
  it('round-trips a built query', () => {
    const q = buildQuery('my-service');
    const parsed = parseQuery(q);
    assert.deepEqual(parsed, {
      service: 'my-service',
      labelSelector: '{service_name="my-service"}',
    });
  });

  it('returns null when service_name is absent', () => {
    assert.equal(parseQuery('{foo="bar"}'), null);
  });

  it('tolerates extra whitespace around operators', () => {
    const parsed = parseQuery('{service_name = "api"}');
    assert.deepEqual(parsed, {
      service: 'api',
      labelSelector: '{service_name = "api"}',
    });
  });

  it('preserves arbitrary label matchers', () => {
    const q = '{service_name="example-service", environment="staging"}';
    const parsed = parseQuery(q);
    assert.deepEqual(parsed, {
      service: 'example-service',
      labelSelector: '{service_name="example-service", environment="staging"}',
    });
  });

  it('handles regex operators and multiple matchers in labelSelector', () => {
    const q = '{service_name="api", region=~"us-.*", env!="prod"}';
    const parsed = parseQuery(q);
    assert.deepEqual(parsed, {
      service: 'api',
      labelSelector: '{service_name="api", region=~"us-.*", env!="prod"}',
    });
  });

  it('extracts service_name when it is not the first matcher', () => {
    const q = '{env="production", service_name="my-app", version="v1.2"}';
    const parsed = parseQuery(q);
    assert.deepEqual(parsed, {
      service: 'my-app',
      labelSelector:
        '{env="production", service_name="my-app", version="v1.2"}',
    });
  });

  it('trims leading and trailing whitespace from labelSelector', () => {
    const q = '  {service_name="api", region="us-east"}  ';
    const parsed = parseQuery(q);
    assert.deepEqual(parsed, {
      service: 'api',
      labelSelector: '{service_name="api", region="us-east"}',
    });
  });

  it('extracts a quoted service_name alongside UTF-8 label names', () => {
    const q = '{"service.namespace"="prod", "service_name"="api"}';
    assert.deepEqual(parseQuery(q), { service: 'api', labelSelector: q });
  });

  it('returns null if service_name uses regex or non-equals operator', () => {
    assert.equal(parseQuery('{service_name=~"api-.*"}'), null);
  });
});
