export default function injectTooltip($: JQueryStatic, wrapperId: string) {
  const tooltipParent = $(`#${wrapperId}`).length
    ? $(`#${wrapperId}`)
    : $(`<div id="${wrapperId}" />`);

  const body = $(`body`);

  return tooltipParent.appendTo(body);
}
