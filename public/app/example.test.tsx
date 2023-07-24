describe('noop', () => {
  it('works', () => {
    expect(true).toBe(true);
  });
});

// TS1208: 'example.test.tsx' cannot be compiled under '--isolatedModules' because it is considered a global script file.
// Add an import, export, or an empty 'export {}' statement to make it a module.
export {};
