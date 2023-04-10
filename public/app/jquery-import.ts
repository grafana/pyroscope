import $ from 'jquery';
interface WindowWithJquery {
  $: unknown;
  jQuery: unknown;
}
(window as unknown as WindowWithJquery).$ = $;
(window as unknown as WindowWithJquery).jQuery = $;
