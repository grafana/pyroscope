// https://github.com/Connormiha/jest-css-modules-transform/issues/33
declare module '*.module.css' {
  const classes: { [key: string]: string };
  export default classes;
}

declare module '*.module.scss' {
  const classes: { [key: string]: string };
  export default classes;
}

declare module '*.module.sass' {
  const classes: { [key: string]: string };
  export default classes;
}

declare module '*.module.less' {
  const classes: { [key: string]: string };
  export default classes;
}

declare module '*.module.styl' {
  const classes: { [key: string]: string };
  export default classes;
}

// https://stackoverflow.com/a/45887328
declare module '*.svg' {
  const content: ShamefulAny;
  export default content;
}

declare module '*.gif' {
  const content: ShamefulAny;
  export default content;
}
