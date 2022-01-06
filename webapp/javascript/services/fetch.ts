const wrap = <T extends Array<any>, U>(fn: (...args: T) => U) => {
  return (...args: T): U => fn(...args);
};
