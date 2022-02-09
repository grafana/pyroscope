/**
  [[include:doc/result.md]]
  
  @module
 */

import type Maybe from './maybe';

import Unit from './unit';
import { curry1, isVoid } from './private/utils';

// Import for backwards-compatibility re-export
import * as Toolbelt from './toolbelt';

/**
  Discriminant for {@linkcode Ok} and {@linkcode Err} variants of the
  {@linkcode Result} type.

  You can use the discriminant via the `variant` property of `Result` instances
  if you need to match explicitly on it.
 */
export const Variant = {
  Ok: 'Ok',
  Err: 'Err',
} as const;

export type Variant = keyof typeof Variant;

export interface OkJSON<T> {
  variant: 'Ok';
  value: T;
}

export interface ErrJSON<E> {
  variant: 'Err';
  error: E;
}

export type ResultJSON<T, E> = OkJSON<T> | ErrJSON<E>;

type Repr<T, E> = [tag: 'Ok', value: T] | [tag: 'Err', error: E];

// Defines the *implementation*, but not the *types*. See the exports below.
class ResultImpl<T, E> {
  private constructor(private repr: Repr<T, E>) {}

  /**
    Create an instance of {@linkcode Ok}.

    Note: While you *may* create the {@linkcode Result} type via normal
    JavaScript class construction, it is not recommended for the functional
    style for which the library is intended. Instead, use {@linkcode ok}.

    ```ts
    // Avoid:
    const aString = new Result.Ok('characters');

    // Prefer:
    const aString = Result.ok('characters);
    ```

    Note that you may explicitly pass {@linkcode Unit.Unit Unit} to the
    {@linkcode Ok} constructor to create a `Result<Unit, E>`. However, you may
    *not* call the `Ok` constructor with `null` or `undefined` to get that
    result (the type system won't allow you to construct it that way). Instead,
    for convenience, you can simply call {@linkcode ok}, which will construct
    the type correctly.

    @param value The value to wrap in an `Ok`.
   */
  static ok<T, E>(): Result<Unit, E>;
  static ok<T, E>(value: T): Result<T, E>;
  static ok<T, E>(value?: T): Result<Unit, E> | Result<T, E> {
    return isVoid(value)
      ? (new ResultImpl<Unit, E>(['Ok', Unit]) as Result<Unit, E>)
      : (new ResultImpl<T, E>(['Ok', value]) as Result<T, E>);
  }

  /**
    Create an instance of {@linkcode Err}.

    Note: While you *may* create the {@linkcode Result} type via normal
    JavaScript class construction, it is not recommended for the functional
    style for which the library is intended. Instead, use {@linkcode err}.

    ```ts
    // Avoid:
    const anErr = new Result.Err('alas, failure');

    // Prefer:
    const anErr = Result.err('alas, failure');
    ```

    @param error The value to wrap in an `Err`.
   */
  static err<T, E>(): Result<T, Unit>;
  static err<T, E>(error: E): Result<T, E>;
  static err<T, E>(error?: E): Result<T, Unit> | Result<T, E> {
    return isVoid(error)
      ? (new ResultImpl<T, Unit>(['Err', Unit]) as Result<T, Unit>)
      : (new ResultImpl<T, E>(['Err', error]) as Result<T, E>);
  }

  /** Distinguish between the {@linkcode Variant.Ok} and {@linkcode Variant.Err} {@linkcode Variant variants}. */
  get variant(): Variant {
    return this.repr[0];
  }

  /**
    The wrapped value.

    @throws if you access when the {@linkcode Result} is not {@linkcode Ok}
   */
  get value(): T | never {
    if (this.repr[0] === Variant.Err) {
      throw new Error('Cannot get the value of Err');
    }

    return this.repr[1];
  }

  /**
    The wrapped error value.

    @throws if you access when the {@linkcode Result} is not {@linkcode Err}
   */
  get error(): E | never {
    if (this.repr[0] === Variant.Ok) {
      throw new Error('Cannot get the error of Ok');
    }

    return this.repr[1];
  }

  /** Is the {@linkcode Result} an {@linkcode Ok}? */
  get isOk() {
    return this.repr[0] === Variant.Ok;
  }

  /** Is the `Result` an `Err`? */
  get isErr() {
    return this.repr[0] === Variant.Err;
  }

  /** Method variant for {@linkcode map} */
  map<U>(this: Result<T, E>, mapFn: (t: T) => U): Result<U, E> {
    return map(mapFn, this);
  }

  /** Method variant for {@linkcode mapOr} */
  mapOr<U>(this: Result<T, E>, orU: U, mapFn: (t: T) => U): U {
    return mapOr(orU, mapFn, this);
  }

  /** Method variant for {@linkcode mapOrElse} */
  mapOrElse<U>(
    this: Result<T, E>,
    orElseFn: (err: E) => U,
    mapFn: (t: T) => U
  ): U {
    return mapOrElse(orElseFn, mapFn, this);
  }

  /** Method variant for {@linkcode match} */
  match<U>(this: Result<T, E>, matcher: Matcher<T, E, U>): U {
    return match(matcher, this);
  }

  /** Method variant for {@linkcode mapErr} */
  mapErr<F>(this: Result<T, E>, mapErrFn: (e: E) => F): Result<T, F> {
    return mapErr(mapErrFn, this);
  }

  /** Method variant for {@linkcode or} */
  or<F>(this: Result<T, E>, orResult: Result<T, F>): Result<T, F> {
    return or(orResult, this);
  }

  /** Method variant for {@linkcode orElse} */
  orElse<F>(
    this: Result<T, E>,
    orElseFn: (err: E) => Result<T, F>
  ): Result<T, F> {
    return orElse(orElseFn, this);
  }

  /** Method variant for {@linkcode and} */
  and<U>(this: Result<T, E>, mAnd: Result<U, E>): Result<U, E> {
    return and(mAnd, this);
  }

  /** Method variant for {@linkcode andThen} */
  andThen<U>(
    this: Result<T, E>,
    andThenFn: (t: T) => Result<U, E>
  ): Result<U, E> {
    return andThen(andThenFn, this);
  }

  /** Method variant for {@linkcode unwrapOr} */
  unwrapOr<U = T>(this: Result<T, E>, defaultValue: U): T | U {
    return unwrapOr(defaultValue, this);
  }

  /** Method variant for {@linkcode unwrapOrElse} */
  unwrapOrElse<U>(this: Result<T, E>, elseFn: (error: E) => U): T | U {
    return unwrapOrElse(elseFn, this);
  }

  /**
    Method variant for {@linkcode Toolbelt.toMaybe toMaybe} from
    {@linkcode Toolbelt}. Prefer to import and use it directly instead:

    ```ts
    import { toMaybe } from 'true-myth/toolbelt';
    ```

    @deprecated until 6.0
   */
  toMaybe(this: Result<T, E>): Maybe<T> {
    return Toolbelt.toMaybe(this);
  }

  /** Method variant for {@linkcode toString} */
  toString(this: Result<T, E>): string {
    return toString(this);
  }

  /** Method variant for {@linkcode toJSON} */
  toJSON(this: Result<T, E>): ResultJSON<T, E> {
    return toJSON(this);
  }

  /** Method variant for {@linkcode equals} */
  equals(this: Result<T, E>, comparison: Result<T, E>): boolean {
    return equals(comparison, this);
  }

  /** Method variant for {@linkcode ap} */
  ap<A, B>(this: Result<(a: A) => B, E>, r: Result<A, E>): Result<B, E> {
    return ap(this, r);
  }
}

/**
  An `Ok` instance is the *successful* variant instance of the
  {@linkcode Result} type, representing a successful outcome from an operation
  which may fail. For a full discussion, see the module docs.

  @typeparam T The type wrapped in this `Ok` variant of `Result`.
  @typeparam E The type which would be wrapped in an `Err` variant of `Result`.
 */
export interface Ok<T, E> extends ResultImpl<T, E> {
  /** `Ok` is always [`Variant.Ok`](../enums/_result_.variant#ok). */
  variant: 'Ok';
  isOk: true;
  isErr: false;
  /** The wrapped value */
  value: T;
  /** @internal */
  error: never;
}

/**
  An `Err` instance is the *failure* variant instance of the {@linkcode Result}
  type, representing a failure outcome from an operation which may fail. For a
  full discussion, see the module docs.

  @typeparam T The type which would be wrapped in an `Ok` variant of `Result`.
  @typeparam E The type wrapped in this `Err` variant of `Result`.
  */
export interface Err<T, E> extends ResultImpl<T, E> {
  /** `Err` is always [`Variant.Err`](../enums/_result_.variant#err). */
  readonly variant: 'Err';
  isOk: false;
  isErr: true;
  /** @internal */
  value: never;
  /** The wrapped error value. */
  error: E;
}

/**
  Execute the provided callback, wrapping the return value in {@linkcode Ok} or
  {@linkcode Err Err(error)} if there is an exception.

  ```ts
  const aSuccessfulOperation = () => 2 + 2;

  const anOkResult = Result.tryOr('Oh noes!!1', () => {
    aSuccessfulOperation()
  }); // => Ok(4)

  const thisOperationThrows = () => throw new Error('Bummer');

  const anErrResult = Result.tryOr('Oh noes!!1', () => {
    thisOperationThrows();
  }); // => Err('Oh noes!!1')
 ```

  @param error The error value in case of an exception
  @param callback The callback to try executing
 */
export function tryOr<T, E>(error: E, callback: () => T): Result<T, E>;
export function tryOr<T, E>(error: E): (callback: () => T) => Result<T, E>;
export function tryOr<T, E>(
  error: E,
  callback?: () => T
): Result<T, E> | ((callback: () => T) => Result<T, E>) {
  const op = (cb: () => T) => {
    try {
      return ok<T, E>(cb());
    } catch {
      return err<T, E>(error);
    }
  };

  return curry1(op, callback);
}

/**
  Create an instance of {@linkcode Ok}.

  If you need to create an instance with a specific type (as you do whenever you
  are not constructing immediately for a function return or as an argument to a
  function), you can use a type parameter:

  ```ts
  const yayNumber = Result.ok<number, string>(12);
  ```

  Note: passing nothing, or passing `null` or `undefined` explicitly, will
  produce a `Result<Unit, E>`, rather than producing the nonsensical and in
  practice quite annoying `Result<null, string>` etc. See {@linkcode Unit} for
  more.

  ```ts
  const normalResult = Result.ok<number, string>(42);
  const explicitUnit = Result.ok<Unit, string>(Unit);
  const implicitUnit = Result.ok<Unit, string>();
  ```

  In the context of an immediate function return, or an arrow function with a
  single expression value, you do not have to specify the types, so this can be
  quite convenient.

  ```ts
  type SomeData = {
    //...
  };

  const isValid = (data: SomeData): boolean => {
    // true or false...
  }

  const arrowValidate = (data: SomeData): Result<Unit, string> =>
    isValid(data) ? Result.ok() : Result.err('something was wrong!');

  function fnValidate(data: someData): Result<Unit, string> {
    return isValid(data) ? Result.ok() : Result.err('something was wrong');
  }
  ```

  @typeparam T The type of the item contained in the `Result`.
  @param value The value to wrap in a `Result.Ok`.
 */
export const ok = ResultImpl.ok;

/**
  Create an instance of {@linkcode Err}.

  If you need to create an instance with a specific type (as you do whenever you
  are not constructing immediately for a function return or as an argument to a
  function), you can use a type parameter:

  ```ts
  const notString = Result.err<number, string>('something went wrong');
  ```

  Note: passing nothing, or passing `null` or `undefined` explicitly, will
  produce a `Result<T, Unit>`, rather than producing the nonsensical and in
  practice quite annoying `Result<null, string>` etc. See {@linkcode Unit} for
  more.

  ```ts
  const normalResult = Result.err<number, string>('oh no');
  const explicitUnit = Result.err<number, Unit>(Unit);
  const implicitUnit = Result.err<number, Unit>();
  ```

  In the context of an immediate function return, or an arrow function with a
  single expression value, you do not have to specify the types, so this can be
  quite convenient.

  ```ts
  type SomeData = {
    //...
  };

  const isValid = (data: SomeData): boolean => {
    // true or false...
  }

  const arrowValidate = (data: SomeData): Result<number, Unit> =>
    isValid(data) ? Result.ok(42) : Result.err();

  function fnValidate(data: someData): Result<number, Unit> {
    return isValid(data) ? Result.ok(42) : Result.err();
  }
  ```

  @typeparam T The type of the item contained in the `Result`.
  @param E The error value to wrap in a `Result.Err`.
 */
export const err = ResultImpl.err;

/**
  Execute the provided callback, wrapping the return value in {@linkcode Ok}.
  If there is an exception, return a {@linkcode Err} of whatever the `onError`
  function returns.

  ```ts
  const aSuccessfulOperation = () => 2 + 2;

  const anOkResult = Result.tryOrElse(
    (e) => e,
    aSuccessfulOperation
  ); // => Ok(4)

  const thisOperationThrows = () => throw 'Bummer'

  const anErrResult = Result.tryOrElse((e) => e, () => {
    thisOperationThrows();
  }); // => Err('Bummer')
 ```

  @param onError A function that takes `e` exception and returns what will
    be wrapped in a `Result.Err`
  @param callback The callback to try executing
 */
export function tryOrElse<T, E>(
  onError: (e: unknown) => E,
  callback: () => T
): Result<T, E>;
export function tryOrElse<T, E>(
  onError: (e: unknown) => E
): (callback: () => T) => Result<T, E>;
export function tryOrElse<T, E>(
  onError: (e: unknown) => E,
  callback?: () => T
): Result<T, E> | ((callback: () => T) => Result<T, E>) {
  const op = (cb: () => T) => {
    try {
      return ok<T, E>(cb());
    } catch (e) {
      return err<T, E>(onError(e));
    }
  };

  return curry1(op, callback);
}

/**
  Map over a {@linkcode Result} instance: apply the function to the wrapped
  value if the instance is {@linkcode Ok}, and return the wrapped error value
  wrapped as a new {@linkcode Err} of the correct type (`Result<U, E>`) if the
  instance is `Err`.

  `map` works a lot like `Array.prototype.map`, but with one important
  difference. Both `Result` and `Array` are containers for other kinds of items,
  but where `Array.prototype.map` has 0 to _n_ items, a `Result` always has
  exactly one item, which is *either* a success or an error instance.

  Where `Array.prototype.map` will apply the mapping function to every item in
  the array (if there are any), `Result.map` will only apply the mapping
  function to the (single) element if an `Ok` instance, if there is one.

  If you have no items in an array of numbers named `foo` and call `foo.map(x =>
  x + 1)`, you'll still some have an array with nothing in it. But if you have
  any items in the array (`[2, 3]`), and you call `foo.map(x => x + 1)` on it,
  you'll get a new array with each of those items inside the array "container"
  transformed (`[3, 4]`).

  With this `map`, the `Err` variant is treated *by the `map` function* kind of
  the same way as the empty array case: it's just ignored, and you get back a
  new `Result` that is still just the same `Err` instance. But if you have an
  `Ok` variant, the map function is applied to it, and you get back a new
  `Result` with the value transformed, and still wrapped in an `Ok`.

  #### Examples

  ```ts
  import { ok, err, map, toString } from 'true-myth/result';
  const double = n => n * 2;

  const anOk = ok(12);
  const mappedOk = map(double, anOk);
  console.log(toString(mappedOk)); // Ok(24)

  const anErr = err("nothing here!");
  const mappedErr = map(double, anErr);
  console.log(toString(mappedOk)); // Err(nothing here!)
  ```

  @typeparam T  The type of the value wrapped in an `Ok` instance, and taken as
                the argument to the `mapFn`.
  @typeparam U  The type of the value wrapped in the new `Ok` instance after
                applying `mapFn`, that is, the type returned by `mapFn`.
  @typeparam E  The type of the value wrapped in an `Err` instance.
  @param mapFn  The function to apply the value to if `result` is `Ok`.
  @param result The `Result` instance to map over.
  @returns      A new `Result` with the result of applying `mapFn` to the value
                in an `Ok`, or else the original `Err` value wrapped in the new
                instance.
 */
export function map<T, U, E>(
  mapFn: (t: T) => U,
  result: Result<T, E>
): Result<U, E>;
export function map<T, U, E>(
  mapFn: (t: T) => U
): (result: Result<T, E>) => Result<U, E>;
export function map<T, U, E>(
  mapFn: (t: T) => U,
  result?: Result<T, E>
): Result<U, E> | ((result: Result<T, E>) => Result<U, E>) {
  const op = (r: Result<T, E>) =>
    (r.isOk ? ok(mapFn(r.value)) : r) as Result<U, E>;
  return curry1(op, result);
}

/**
  Map over a {@linkcode Result} instance as in [`map`](#map) and get out the
  value if `result` is an {@linkcode Ok}, or return a default value if `result`
  is an {@linkcode Err}.

  #### Examples

  ```ts
  import { ok, err, mapOr } from 'true-myth/result';

  const length = (s: string) => s.length;

  const anOkString = ok('a string');
  const theStringLength = mapOr(0, anOkString);
  console.log(theStringLength);  // 8

  const anErr = err('uh oh');
  const anErrMapped = mapOr(0, anErr);
  console.log(anErrMapped);  // 0
  ```

  @param orU The default value to use if `result` is an `Err`.
  @param mapFn The function to apply the value to if `result` is an `Ok`.
  @param result The `Result` instance to map over.
 */
export function mapOr<T, U, E>(
  orU: U,
  mapFn: (t: T) => U,
  result: Result<T, E>
): U;
export function mapOr<T, U, E>(
  orU: U,
  mapFn: (t: T) => U
): (result: Result<T, E>) => U;
export function mapOr<T, U, E>(
  orU: U
): (mapFn: (t: T) => U) => (result: Result<T, E>) => U;
export function mapOr<T, U, E>(
  orU: U,
  mapFn?: (t: T) => U,
  result?: Result<T, E>
):
  | U
  | ((result: Result<T, E>) => U)
  | ((mapFn: (t: T) => U) => (result: Result<T, E>) => U) {
  function fullOp(fn: (t: T) => U, r: Result<T, E>): U {
    return r.isOk ? fn(r.value) : orU;
  }

  function partialOp(fn: (t: T) => U): (maybe: Result<T, E>) => U;
  function partialOp(fn: (t: T) => U, curriedResult: Result<T, E>): U;
  function partialOp(
    fn: (t: T) => U,
    curriedResult?: Result<T, E>
  ): U | ((maybe: Result<T, E>) => U) {
    return curriedResult !== undefined
      ? fullOp(fn, curriedResult)
      : (extraCurriedResult: Result<T, E>) => fullOp(fn, extraCurriedResult);
  }

  return mapFn === undefined
    ? partialOp
    : result === undefined
    ? partialOp(mapFn)
    : partialOp(mapFn, result);
}

/**
  Map over a {@linkcode Result} instance as in {@linkcode map} and get out the
  value if `result` is {@linkcode Ok}, or apply a function (`orElseFn`) to the
  value wrapped in the {@linkcode Err} to get a default value.

  Like {@linkcode mapOr} but using a function to transform the error into a
  usable value instead of simply using a default value.

  #### Examples

  ```ts
  import { ok, err, mapOrElse } from 'true-myth/result';

  const summarize = (s: string) => `The response was: '${s}'`;
  const getReason = (err: { code: number, reason: string }) => err.reason;

  const okResponse = ok("Things are grand here.");
  const mappedOkAndUnwrapped = mapOrElse(getReason, summarize, okResponse);
  console.log(mappedOkAndUnwrapped);  // The response was: 'Things are grand here.'

  const errResponse = err({ code: 500, reason: 'Nothing at this endpoint!' });
  const mappedErrAndUnwrapped = mapOrElse(getReason, summarize, errResponse);
  console.log(mappedErrAndUnwrapped);  // Nothing at this endpoint!
  ```

  @typeparam T    The type of the wrapped `Ok` value.
  @typeparam U    The type of the resulting value from applying `mapFn` to the
                  `Ok` value or `orElseFn` to the `Err` value.
  @typeparam E    The type of the wrapped `Err` value.
  @param orElseFn The function to apply to the wrapped `Err` value to get a
                  usable value if `result` is an `Err`.
  @param mapFn    The function to apply to the wrapped `Ok` value if `result` is
                  an `Ok`.
  @param result   The `Result` instance to map over.
 */
export function mapOrElse<T, U, E>(
  orElseFn: (err: E) => U,
  mapFn: (t: T) => U,
  result: Result<T, E>
): U;
export function mapOrElse<T, U, E>(
  orElseFn: (err: E) => U,
  mapFn: (t: T) => U
): (result: Result<T, E>) => U;
export function mapOrElse<T, U, E>(
  orElseFn: (err: E) => U
): (mapFn: (t: T) => U) => (result: Result<T, E>) => U;
export function mapOrElse<T, U, E>(
  orElseFn: (err: E) => U,
  mapFn?: (t: T) => U,
  result?: Result<T, E>
):
  | U
  | ((result: Result<T, E>) => U)
  | ((mapFn: (t: T) => U) => (result: Result<T, E>) => U) {
  function fullOp(fn: (t: T) => U, r: Result<T, E>) {
    return r.isOk ? fn(r.value) : orElseFn(r.error);
  }

  function partialOp(fn: (t: T) => U): (maybe: Result<T, E>) => U;
  function partialOp(fn: (t: T) => U, curriedResult: Result<T, E>): U;
  function partialOp(
    fn: (t: T) => U,
    curriedResult?: Result<T, E>
  ): U | ((maybe: Result<T, E>) => U) {
    return curriedResult !== undefined
      ? fullOp(fn, curriedResult)
      : (extraCurriedResult: Result<T, E>) => fullOp(fn, extraCurriedResult);
  }

  return mapFn === undefined
    ? partialOp
    : result === undefined
    ? partialOp(mapFn)
    : partialOp(mapFn, result);
}

/**
  Map over a {@linkcode Ok}, exactly as in {@linkcode map}, but operating on the
  value wrapped in an {@linkcode Err} instead of the value wrapped in the
  {@linkcode Ok}. This is handy for when you need to line up a bunch of
  different types of errors, or if you need an error of one shape to be in a
  different shape to use somewhere else in your codebase.

  #### Examples

  ```ts
  import { ok, err, mapErr, toString } from 'true-myth/result';

  const reason = (err: { code: number, reason: string }) => err.reason;

  const anOk = ok(12);
  const mappedOk = mapErr(reason, anOk);
  console.log(toString(mappedOk));  // Ok(12)

  const anErr = err({ code: 101, reason: 'bad file' });
  const mappedErr = mapErr(reason, anErr);
  console.log(toString(mappedErr));  // Err(bad file)
  ```

  @typeparam T    The type of the value wrapped in the `Ok` of the `Result`.
  @typeparam E    The type of the value wrapped in the `Err` of the `Result`.
  @typeparam F    The type of the value wrapped in the `Err` of a new `Result`,
                  returned by the `mapErrFn`.
  @param mapErrFn The function to apply to the value wrapped in `Err` if
  `result` is an `Err`.
  @param result   The `Result` instance to map over an error case for.
 */
export function mapErr<T, E, F>(
  mapErrFn: (e: E) => F,
  result: Result<T, E>
): Result<T, F>;
export function mapErr<T, E, F>(
  mapErrFn: (e: E) => F
): (result: Result<T, E>) => Result<T, F>;
export function mapErr<T, E, F>(
  mapErrFn: (e: E) => F,
  result?: Result<T, E>
): Result<T, F> | ((result: Result<T, E>) => Result<T, F>) {
  const op = (r: Result<T, E>) =>
    (r.isOk ? r : err(mapErrFn(r.error))) as Result<T, F>;
  return curry1(op, result);
}

/**
  You can think of this like a short-circuiting logical "and" operation on a
  {@linkcode Result} type. If `result` is {@linkcode Ok}, then the result is the
  `andResult`. If `result` is {@linkcode Err}, the result is the `Err`.

  This is useful when you have another `Result` value you want to provide if and
  *only if* you have an `Ok` – that is, when you need to make sure that if you
  `Err`, whatever else you're handing a `Result` to *also* gets that `Err`.

  Notice that, unlike in [`map`](#map) or its variants, the original `result` is
  not involved in constructing the new `Result`.

  #### Examples

  ```ts
  import { and, ok, err, toString } from 'true-myth/result';

  const okA = ok('A');
  const okB = ok('B');
  const anErr = err({ so: 'bad' });

  console.log(toString(and(okB, okA)));  // Ok(B)
  console.log(toString(and(okB, anErr)));  // Err([object Object])
  console.log(toString(and(anErr, okA)));  // Err([object Object])
  console.log(toString(and(anErr, anErr)));  // Err([object Object])
  ```

  @typeparam T     The type of the value wrapped in the `Ok` of the `Result`.
  @typeparam U     The type of the value wrapped in the `Ok` of the `andResult`,
                   i.e. the success type of the `Result` present if the checked
                   `Result` is `Ok`.
  @typeparam E     The type of the value wrapped in the `Err` of the `Result`.
  @param andResult The `Result` instance to return if `result` is `Err`.
  @param result    The `Result` instance to check.
 */
export function and<T, U, E>(
  andResult: Result<U, E>,
  result: Result<T, E>
): Result<U, E>;
export function and<T, U, E>(
  andResult: Result<U, E>
): (result: Result<T, E>) => Result<U, E>;
export function and<T, U, E>(
  andResult: Result<U, E>,
  result?: Result<T, E>
): Result<U, E> | ((result: Result<T, E>) => Result<U, E>) {
  const op = (r: Result<T, E>) => (r.isOk ? andResult : err<U, E>(r.error));
  return curry1(op, result);
}

/**
  Apply a function to the wrapped value if {@linkcode Ok} and return a new `Ok`
  containing the resulting value; or if it is {@linkcode Err} return it
  unmodified.

  This differs from `map` in that `thenFn` returns another {@linkcode Result}.
  You can use `andThen` to combine two functions which *both* create a `Result`
  from an unwrapped type.

  You may find the `.then` method on an ES6 `Promise` helpful for comparison: if
  you have a `Promise`, you can pass its `then` method a callback which returns
  another `Promise`, and the result will not be a *nested* promise, but a single
  `Promise`. The difference is that `Promise#then` unwraps *all* layers to only
  ever return a single `Promise` value, whereas `Result.andThen` will not unwrap
  nested `Result`s.

  This is is sometimes also known as `bind`, but *not* aliased as such because
  [`bind` already means something in JavaScript][bind].

  [bind]: https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Function/bind

  #### Examples

  ```ts
  import { ok, err, andThen, toString } from 'true-myth/result';

  const toLengthAsResult = (s: string) => ok(s.length);

  const anOk = ok('just a string');
  const lengthAsResult = andThen(toLengthAsResult, anOk);
  console.log(toString(lengthAsResult));  // Ok(13)

  const anErr = err(['srsly', 'whatever']);
  const notLengthAsResult = andThen(toLengthAsResult, anErr);
  console.log(toString(notLengthAsResult));  // Err(srsly,whatever)
  ```

  @typeparam T   The type of the value wrapped in the `Ok` of the `Result`.
  @typeparam U   The type of the value wrapped in the `Ok` of the `Result`
                 returned by the `thenFn`.
  @typeparam E   The type of the value wrapped in the `Err` of the `Result`.
  @param thenFn  The function to apply to the wrapped `T` if `maybe` is `Just`.
  @param result  The `Maybe` to evaluate and possibly apply a function to.
 */
export function andThen<T, U, E>(
  thenFn: (t: T) => Result<U, E>,
  result: Result<T, E>
): Result<U, E>;
export function andThen<T, U, E>(
  thenFn: (t: T) => Result<U, E>
): (result: Result<T, E>) => Result<U, E>;
export function andThen<T, U, E>(
  thenFn: (t: T) => Result<U, E>,
  result?: Result<T, E>
): Result<U, E> | ((result: Result<T, E>) => Result<U, E>) {
  const op = (r: Result<T, E>) =>
    r.isOk ? thenFn(r.value) : err<U, E>(r.error);
  return curry1(op, result);
}

/**
  Provide a fallback for a given {@linkcode Result}. Behaves like a logical
  `or`: if the `result` value is an {@linkcode Ok}, returns that `result`;
  otherwise, returns the `defaultResult` value.

  This is useful when you want to make sure that something which takes a
  `Result` always ends up getting an `Ok` variant, by supplying a default value
  for the case that you currently have an {@linkcode Err}.

  ```ts
  import { ok, err, Result, or } from 'true-utils/result';

  const okA = ok<string, string>('a');
  const okB = ok<string, string>('b');
  const anErr = err<string, string>(':wat:');
  const anotherErr = err<string, string>(':headdesk:');

  console.log(or(okB, okA).toString());  // Ok(A)
  console.log(or(anErr, okA).toString());  // Ok(A)
  console.log(or(okB, anErr).toString());  // Ok(B)
  console.log(or(anotherErr, anErr).toString());  // Err(:headdesk:)
  ```

  @typeparam T          The type wrapped in the `Ok` case of `result`.
  @typeparam E          The type wrapped in the `Err` case of `result`.
  @typeparam F          The type wrapped in the `Err` case of `defaultResult`.
  @param defaultResult  The `Result` to use if `result` is an `Err`.
  @param result         The `Result` instance to check.
  @returns              `result` if it is an `Ok`, otherwise `defaultResult`.
 */
export function or<T, E, F>(
  defaultResult: Result<T, F>,
  result: Result<T, E>
): Result<T, F>;
export function or<T, E, F>(
  defaultResult: Result<T, F>
): (result: Result<T, E>) => Result<T, F>;
export function or<T, E, F>(
  defaultResult: Result<T, F>,
  result?: Result<T, E>
): Result<T, F> | ((result: Result<T, E>) => Result<T, F>) {
  const op = (r: Result<T, E>) => (r.isOk ? ok<T, F>(r.value) : defaultResult);
  return curry1(op, result);
}

/**
  Like {@linkcode or}, but using a function to construct the alternative
  {@linkcode Result}.

  Sometimes you need to perform an operation using other data in the environment
  to construct the fallback value. In these situations, you can pass a function
  (which may be a closure) as the `elseFn` to generate the fallback `Result<T>`.
  It can then transform the data in the `Err` to something usable as an
  {@linkcode Ok}, or generate a new {@linkcode Err} instance as appropriate.

  Useful for transforming failures to usable data.

  @param elseFn The function to apply to the contents of the `Err` if `result`
                is an `Err`, to create a new `Result`.
  @param result The `Result` to use if it is an `Ok`.
  @returns      The `result` if it is `Ok`, or the `Result` returned by `elseFn`
                if `result` is an `Err.
 */
export function orElse<T, E, F>(
  elseFn: (err: E) => Result<T, F>,
  result: Result<T, E>
): Result<T, F>;
export function orElse<T, E, F>(
  elseFn: (err: E) => Result<T, F>
): (result: Result<T, E>) => Result<T, F>;
export function orElse<T, E, F>(
  elseFn: (err: E) => Result<T, F>,
  result?: Result<T, E>
): Result<T, F> | ((result: Result<T, E>) => Result<T, F>) {
  const op = (r: Result<T, E>) =>
    r.isOk ? ok<T, F>(r.value) : elseFn(r.error);
  return curry1(op, result);
}

/**
  Safely get the value out of the {@linkcode Ok} variant of a {@linkcode Result}.

  This is the recommended way to get a value out of a `Result` most of the time.

  ```ts
  import { ok, err, unwrapOr } from 'true-myth/result';

  const anOk = ok<number, string>(12);
  console.log(unwrapOr(0, anOk));  // 12

  const anErr = err<number, string>('nooooo');
  console.log(unwrapOr(0, anErr));  // 0
  ```

  @typeparam T        The value wrapped in the `Ok`.
  @typeparam E        The value wrapped in the `Err`.
  @param defaultValue The value to use if `result` is an `Err`.
  @param result       The `Result` instance to unwrap if it is an `Ok`.
  @returns            The content of `result` if it is an `Ok`, otherwise
                      `defaultValue`.
 */
export function unwrapOr<T, U, E>(defaultValue: U, result: Result<T, E>): U | T;
export function unwrapOr<T, U, E>(
  defaultValue: U
): (result: Result<T, E>) => U | T;
export function unwrapOr<T, U, E>(
  defaultValue: U,
  result?: Result<T, E>
): (T | U) | ((result: Result<T, E>) => T | U) {
  const op = (r: Result<T, E>) => (r.isOk ? r.value : defaultValue);
  return curry1(op, result);
}

/**
  Safely get the value out of a {@linkcode Result} by returning the wrapped
  value if it is {@linkcode Ok}, or by applying `orElseFn` to the value in the
  {@linkcode Err}.

  This is useful when you need to *generate* a value (e.g. by using current
  values in the environment – whether preloaded or by local closure) instead of
  having a single default value available (as in {@linkcode unwrapOr}).

  ```ts
  import { ok, err, unwrapOrElse } from 'true-myth/result';

  // You can imagine that someOtherValue might be dynamic.
  const someOtherValue = 2;
  const handleErr = (errValue: string) => errValue.length + someOtherValue;

  const anOk = ok<number, string>(42);
  console.log(unwrapOrElse(handleErr, anOk));  // 42

  const anErr = err<number, string>('oh teh noes');
  console.log(unwrapOrElse(handleErr, anErr));  // 13
  ```

  @typeparam T    The value wrapped in the `Ok`.
  @typeparam E    The value wrapped in the `Err`.
  @param orElseFn A function applied to the value wrapped in `result` if it is
                  an `Err`, to generate the final value.
  @param result   The `result` to unwrap if it is an `Ok`.
  @returns        The value wrapped in `result` if it is `Ok` or the value
                  returned by `orElseFn` applied to the value in `Err`.
 */
export function unwrapOrElse<T, U, E>(
  orElseFn: (error: E) => U,
  result: Result<T, E>
): T | U;
export function unwrapOrElse<T, U, E>(
  orElseFn: (error: E) => U
): (result: Result<T, E>) => T | U;
export function unwrapOrElse<T, U, E>(
  orElseFn: (error: E) => U,
  result?: Result<T, E>
): (T | U) | ((result: Result<T, E>) => T | U) {
  const op = (r: Result<T, E>) => (r.isOk ? r.value : orElseFn(r.error));
  return curry1(op, result);
}

/**
  Local implementation of {@linkcode Toolbelt.toOkOrErr toOkOrErr} from
  {@linkcode Toolbelt} for backwards compatibility. Prefer to import it from
  there instead:

  ```ts
  import type { toOkOrErr } from 'true-myth/toolbelt';
  ```

  @deprecated until 6.0
 */
export function fromMaybe<T, E>(errValue: E, maybe: Maybe<T>): Result<T, E>;
export function fromMaybe<T, E>(errValue: E): (maybe: Maybe<T>) => Result<T, E>;
export function fromMaybe<T, E>(
  errValue: E,
  maybe?: Maybe<T>
): Result<T, E> | ((maybe: Maybe<T>) => Result<T, E>) {
  const op = (m: Maybe<T>) =>
    m.isJust ? ok<T, E>(m.value) : err<T, E>(errValue);
  return curry1(op, maybe);
}

/**
  Create a `String` representation of a {@linkcode Result} instance.

  An {@linkcode Ok} instance will be `Ok(<representation of the value>)`, and an
  {@linkcode Err} instance will be `Err(<representation of the error>)`, where
  the representation of the value or error is simply the value or error's own
  `toString` representation. For example:

                call                |         output
  --------------------------------- | ----------------------
  `toString(ok(42))`                | `Ok(42)`
  `toString(ok([1, 2, 3]))`         | `Ok(1,2,3)`
  `toString(ok({ an: 'object' }))`  | `Ok([object Object])`n
  `toString(err(42))`               | `Err(42)`
  `toString(err([1, 2, 3]))`        | `Err(1,2,3)`
  `toString(err({ an: 'object' }))` | `Err([object Object])`

  @typeparam T The type of the wrapped value; its own `.toString` will be used
               to print the interior contents of the `Just` variant.
  @param maybe The value to convert to a string.
  @returns     The string representation of the `Maybe`.
 */
export const toString = <
  T extends { toString(): string },
  E extends { toString(): string }
>(
  result: Result<T, E>
): string => {
  const body = (result.isOk ? result.value : result.error).toString();
  return `${result.variant.toString()}(${body})`;
};

/**
 * Create an `Object` representation of a {@linkcode Result} instance.
 *
 * Useful for serialization. `JSON.stringify()` uses it.
 *
 * @param result  The value to convert to JSON
 * @returns       The JSON representation of the `Result`
 */
export const toJSON = <T, E>(result: Result<T, E>): ResultJSON<T, E> => {
  return result.isOk
    ? { variant: result.variant, value: result.value }
    : { variant: result.variant, error: result.error };
};

/**
  A lightweight object defining how to handle each variant of a
  {@linkcode Result}.
 */
export type Matcher<T, E, A> = {
  Ok: (value: T) => A;
  Err: (error: E) => A;
};

/**
  Performs the same basic functionality as {@linkcode unwrapOrElse}, but instead
  of simply unwrapping the value if it is {@linkcode Ok} and applying a value to
  generate the same default type if it is {@linkcode Err}, lets you supply
  functions which may transform the wrapped type if it is `Ok` or get a default
  value for `Err`.

  This is kind of like a poor man's version of pattern matching, which
  JavaScript currently lacks.

  Instead of code like this:

  ```ts
  import Result, { isOk, match } from 'true-myth/result';

  const logValue = (mightBeANumber: Result<number, string>) => {
    console.log(
      isOk(mightBeANumber)
        ? unsafelyUnwrap(mightBeANumber).toString()
        : `There was an error: ${unsafelyGetErr(mightBeANumber)}`
    );
  };
  ```

  ...we can write code like this:

  ```ts
  import Result, { match } from 'true-myth/result';

  const logValue = (mightBeANumber: Result<number, string>) => {
    const value = match(
      {
        Ok: n => n.toString(),
        Err: e => `There was an error: ${e}`,
      },
      mightBeANumber
    );
    console.log(value);
  };
  ```

  This is slightly longer to write, but clearer: the more complex the resulting
  expression, the hairer it is to understand the ternary. Thus, this is
  especially convenient for times when there is a complex result, e.g. when
  rendering part of a React component inline in JSX/TSX.

  @param matcher A lightweight object defining what to do in the case of each
                 variant.
  @param maybe   The `maybe` instance to check.
 */
export function match<T, E, A>(
  matcher: Matcher<T, E, A>,
  result: Result<T, E>
): A;
export function match<T, E, A>(
  matcher: Matcher<T, E, A>
): (result: Result<T, E>) => A;
export function match<T, E, A>(
  matcher: Matcher<T, E, A>,
  result?: Result<T, E>
): A | ((result: Result<T, E>) => A) {
  const op = (r: Result<T, E>) => mapOrElse(matcher.Err, matcher.Ok, r);
  return curry1(op, result);
}

/**
  Allows quick triple-equal equality check between the values inside two
  {@linkcode Result}s without having to unwrap them first.

  ```ts
  const a = Result.of(3)
  const b = Result.of(3)
  const c = Result.of(null)
  const d = Result.nothing()

  Result.equals(a, b) // true
  Result.equals(a, c) // false
  Result.equals(c, d) // true
  ```

  @param resultB A `maybe` to compare to.
  @param resultA A `maybe` instance to check.
 */
export function equals<T, E>(
  resultB: Result<T, E>,
  resultA: Result<T, E>
): boolean;
export function equals<T, E>(
  resultB: Result<T, E>
): (resultA: Result<T, E>) => boolean;
export function equals<T, E>(
  resultB: Result<T, E>,
  resultA?: Result<T, E>
): boolean | ((a: Result<T, E>) => boolean) {
  return resultA !== undefined
    ? resultA.match({
        Err: () => resultB.isErr,
        Ok: (a) => resultB.isOk && resultB.value === a,
      })
    : (curriedResultA: Result<T, E>) =>
        curriedResultA.match({
          Err: () => resultB.isErr,
          Ok: (a) => resultB.isOk && resultB.value === a,
        });
}

/**
  Allows you to *apply* (thus `ap`) a value to a function without having to take
  either out of the context of their {@linkcode Result}s. This does mean that
  the transforming function is itself within a `Result`, which can be hard to
  grok at first but lets you do some very elegant things. For example, `ap`
  allows you to do this (using the method form, since nesting `ap` calls is
  awkward):

  ```ts
  import { ap, ok, err } from 'true-myth/result';

  const one = ok<number, string>(1);
  const five = ok<number, string>(5);
  const whoops = err<number, string>('oh no');

  const add = (a: number) => (b: number) => a + b;
  const resultAdd = ok<typeof add, string>(add);

  resultAdd.ap(one).ap(five); // Ok(6)
  resultAdd.ap(one).ap(whoops); // Err('oh no')
  resultAdd.ap(whoops).ap(five) // Err('oh no')
  ```

  Without `ap`, you'd need to do something like a nested `match`:

  ```ts
  import { ok, err } from 'true-myth/result';

  const one = ok<number, string>(1);
  const five = ok<number, string>(5);
  const whoops = err<number, string>('oh no');

  one.match({
    Ok: n => five.match({
      Ok: o => ok<number, string>(n + o),
      Err: e => err<number, string>(e),
    }),
    Err: e  => err<number, string>(e),
  }); // Ok(6)

  one.match({
    Ok: n => whoops.match({
      Ok: o => ok<number, string>(n + o),
      Err: e => err<number, string>(e),
    }),
    Err: e  => err<number, string>(e),
  }); // Err('oh no')

  whoops.match({
    Ok: n => five.match({
      Ok: o => ok(n + o),
      Err: e => err(e),
    }),
    Err: e  => err(e),
  }); // Err('oh no')
  ```

  And this kind of thing comes up quite often once you're using `Result` to
  handle errors throughout your application.

  For another example, imagine you need to compare the equality of two
  ImmutableJS data structures, where a `===` comparison won't work. With `ap`,
  that's as simple as this:

  ```ts
  import { ok } from 'true-myth/result';
  import { is as immutableIs, Set } from 'immutable';

  const is = (first: unknown) =>  (second: unknown) => 
    immutableIs(first, second);

  const x = ok(Set.of(1, 2, 3));
  const y = ok(Set.of(2, 3, 4));

  ok(is).ap(x).ap(y); // Ok(false)
  ```

  Without `ap`, we're back to that gnarly nested `match`:

  ```ts
  import Result, { ok, err } from 'true-myth/result';
  import { is, Set } from 'immutable';

  const x = ok(Set.of(1, 2, 3));
  const y = ok(Set.of(2, 3, 4));

  x.match({
    Ok: iX => y.match({
      Ok: iY => Result.of(is(iX, iY)),
      Err: (e) => ok(false),
    })
    Err: (e) => ok(false),
  }); // Ok(false)
  ```

  In summary: anywhere you have two `Result` instances and need to perform an
  operation that uses both of them, `ap` is your friend.

  Two things to note, both regarding *currying*:

  1.  All functions passed to `ap` must be curried. That is, they must be of the
      form (for add) `(a: number) => (b: number) => a + b`, *not* the more usual
      `(a: number, b: number) => a + b` you see in JavaScript more generally.

      (Unfortunately, these do not currently work with lodash or Ramda's `curry`
      helper functions. A future update to the type definitions may make that
      work, but the intermediate types produced by those helpers and the more
      general function types expected by this function do not currently align.)

  2.  You will need to call `ap` as many times as there are arguments to the
      function you're dealing with. So in the case of this `add3` function,
      which has the "arity" (function argument count) of 3 (`a` and `b`), you'll
      need to call `ap` twice: once for `a`, and once for `b`. To see why, let's
      look at what the result in each phase is:

      ```ts
      const add3 = (a: number) => (b: number) => (c: number) => a + b + c;

      const resultAdd = ok(add); // Ok((a: number) => (b: number) => (c: number) => a + b + c)
      const resultAdd1 = resultAdd.ap(ok(1)); // Ok((b: number) => (c: number) => 1 + b + c)
      const resultAdd1And2 = resultAdd1.ap(ok(2)) // Ok((c: number) => 1 + 2 + c)
      const final = maybeAdd1.ap(ok(3)); // Ok(4)
      ```

      So for `toString`, which just takes a single argument, you would only need
      to call `ap` once.

      ```ts
      const toStr = (v: { toString(): string }) => v.toString();
      ok(toStr).ap(12); // Ok("12")
      ```

  One other scenario which doesn't come up *quite* as often but is conceivable
  is where you have something that may or may not actually construct a function
  for handling a specific `Result` scenario. In that case, you can wrap the
  possibly-present in `ap` and then wrap the values to apply to the function to
  in `Result` themselves.

  Because `Result` often requires you to type out the full type parameterization
  on a regular basis, it's convenient to use TypeScript's `typeof` operator to
  write out the type of a curried function. For example, if you had a function
  that simply merged three strings, you might write it like this:

  ```ts
  import Result from 'true-myth/result';
  import { curry } from 'lodash';

  const merge3Strs = (a: string, b: string, c: string) => string;
  const curriedMerge = curry(merge3Strs);

  const fn = Result.ok<typeof curriedMerge, string>(curriedMerge);
  ```

  The alternative is writing out the full signature long-form:

  ```ts
  const fn = Result.ok<(a: string) => (b: string) => (c: string) => string, string>(curriedMerge);
  ```

  **Aside:** `ap` is not named `apply` because of the overlap with JavaScript's
  existing [`apply`] function – and although strictly speaking, there isn't any
  direct overlap (`Result.apply` and `Function.prototype.apply` don't intersect
  at all) it's useful to have a different name to avoid implying that they're
  the same.

  [`apply`]: https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Function/apply

  @param resultFn result of a function from T to U
  @param result result of a T to apply to `fn`
 */
export function ap<T, U, E>(
  resultFn: Result<(t: T) => U, E>,
  result: Result<T, E>
): Result<U, E>;
export function ap<T, U, E>(
  resultFn: Result<(t: T) => U, E>
): (result: Result<T, E>) => Result<U, E>;
export function ap<T, U, E>(
  resultFn: Result<(val: T) => U, E>,
  result?: Result<T, E>
): Result<U, E> | ((val: Result<T, E>) => Result<U, E>) {
  const op = (r: Result<T, E>) =>
    r.andThen((val) => resultFn.map((fn) => fn(val)));
  return curry1(op, result);
}

/**
  Determine whether an item is an instance of {@linkcode Result}.

  @param item The item to check.
 */
export function isInstance<T, E>(item: unknown): item is Result<T, E> {
  return item instanceof ResultImpl;
}

/**
  Re-export of {@linkcode Toolbelt.transposeMaybe transposeMaybe} from
  {@linkcode Toolbelt} for backwards compatibility.Prefer to import it from
  there instead:

  ```ts
  import type { transposeMaybe } from 'true-myth/toolbelt';
  ```

  @deprecated until 6.0
 */
export function transposeMaybe<T, E>(maybe: Maybe<Result<T, E>>) {
  return Toolbelt.transposeMaybe(maybe);
}

/**
  Re-export of {@linkcode Toolbelt.toMaybe toMaybe} from
  {@linkcode Toolbelt} for backwards compatibility.Prefer to import it from
  there instead:

  ```ts
  import type { toMaybe } from 'true-myth/toolbelt';
  ```

  @deprecated until 6.0
 */
export function toMaybe<T, E>(result: Result<T, E>): Maybe<T> {
  return Toolbelt.toMaybe(result);
}

// The public interface for the {@linkcode Result} class *as a value*: a constructor and the
// single associated static property.
export interface ResultConstructor {
  ok: typeof ResultImpl.ok;
  err: typeof ResultImpl.err;
}

/**
  A value which may ({@linkcode Ok}) or may not ({@linkcode Err}) be present.

  The behavior of this type is checked by TypeScript at compile time, and bears
  no runtime overhead other than the very small cost of the container object.
 */
export type Result<T, E> = Ok<T, E> | Err<T, E>;
export const Result = ResultImpl as ResultConstructor;
export default Result;
