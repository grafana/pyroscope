import jquery from 'jquery';

interface Window {
  jQuery?: unknown;
  $?: unknown;
}

// Used by react-flot/flotjs
(window as Window).jQuery = jquery;
(window as Window).$ = jquery;
