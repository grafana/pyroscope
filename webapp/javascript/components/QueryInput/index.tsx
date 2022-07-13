import React, {
  useState,
  useEffect,
  useRef,
  Dispatch,
  SetStateAction,
} from 'react';
import Button from '@webapp/ui/Button';
import ReactDOM from 'react-dom';
import { useWindowWidth } from '@react-hook/window-size';
import TextareaAutosize from 'react-textarea-autosize';
import { Prism } from '@webapp/util/prism';
import { Query, brandQuery } from '@webapp/models/query';
import Menu from './Menu';
import styles from './QueryInput.module.scss';

const inlineSvg = `<svg viewPort="0 0 12 12" version="1.1" xmlns="http://www.w3.org/2000/svg"> <line x1="1" y1="11" x2="11" y2="1" stroke-width="2"/> <line x1="1" y1="1" x2="11" y2="11" stroke-width="2"/></svg>`;

interface QueryInputProps {
  initialQuery: Query;
  onSubmit: (query: string) => void;
}

const createElem = (
  type: 'span',
  role: string,
  className: string,
  innerHTML?: string
) => {
  const el = document.createElement(type);
  el.className = className;
  el.dataset.role = role;

  if (innerHTML) {
    el.innerHTML = innerHTML;
  }

  return el;
};

export const getInnerText = (elem: HTMLElement | null | undefined) => {
  return elem?.innerText as string;
};

function decorateAttrValues(
  setQuery: Dispatch<SetStateAction<string>>,
  query: string
) {
  Prism.hooks.add('after-highlight', (env) => {
    const list = env.element.querySelectorAll(
      'span.token.label-value.attr-value'
    );

    const handleClickClose = (e: MouseEvent) => {
      const eTarget = e.target as HTMLElement;
      const attrValue = getInnerText(eTarget?.parentElement);
      const attrName = getInnerText(
        eTarget?.parentElement?.previousElementSibling
          ?.previousElementSibling as HTMLElement
      );

      const regExp = new RegExp(
        `\\b${attrName}\\b[\\s|=|~|!]{1,4}["'\`](.*?(${attrValue.replace(
          /['"\s]+/g,
          ''
        )}).*?)["'\`]`,
        'gmi'
      );

      const matched = query.match(regExp);

      const result = matched?.[0] ? query.replace(matched?.[0], '') : query;

      setQuery(result);
    };

    Array.from(list).forEach((item) => {
      const closeBox =
        item.querySelector(`span[data-role="close"]`) ||
        createElem('span', 'close', styles.deletePill, inlineSvg);

      (closeBox as HTMLSpanElement).onclick = handleClickClose;

      item.appendChild(closeBox);

      const menuBox =
        item.querySelector(`span[data-role="menu"]`) ||
        createElem('span', 'menu', styles.itemMenuWrapper);

      ReactDOM.render(<Menu parent={menuBox} query={query} />, menuBox);

      item.appendChild(menuBox);
    });
  });
}

function QueryInput({ initialQuery, onSubmit }: QueryInputProps) {
  const windowWidth = useWindowWidth();
  const [query, setQuery] = useState<string>(initialQuery);
  const codeRef = useRef(null);
  const textareaRef = useRef(null);
  const [textAreaSize, setTextAreaSize] = useState({ width: 0, height: 0 });

  // If query updated upstream, most likely the application changed
  // So we prefer to use it
  useEffect(() => {
    setQuery(initialQuery);
  }, [initialQuery]);

  useEffect(() => {
    // execute decoration before highlighting
    decorateAttrValues(setQuery, query);

    if (Prism && codeRef.current) {
      Prism.highlightElement(codeRef.current);
    }
  }, [query, setQuery]);

  useEffect(() => {
    setTextAreaSize({
      width: textareaRef?.current?.['offsetWidth'] || 0,
      height: textareaRef?.current?.['offsetHeight'] || 0,
    });
  }, [query, windowWidth, onSubmit]);

  const onFormSubmit = (
    e:
      | React.FormEvent<HTMLFormElement>
      | React.KeyboardEvent<HTMLTextAreaElement>
  ) => {
    e.preventDefault();
    onSubmit(query);
  };

  const handleTextareaKeyDown = (
    e: React.KeyboardEvent<HTMLTextAreaElement>
  ) => {
    if ((e.keyCode === 10 || e.keyCode === 13) && e.ctrlKey) {
      onFormSubmit(e);
    }
  };

  return (
    <form className="tags-query" onSubmit={onFormSubmit}>
      <TextareaAutosize
        className="tags-input"
        ref={textareaRef}
        value={query}
        spellCheck="false"
        onChange={(e) => setQuery(brandQuery(e.target.value))}
        onKeyDown={handleTextareaKeyDown}
      />
      <pre
        style={textAreaSize}
        ref={codeRef}
        className="tags-highlighted language-promql"
        aria-hidden="true"
      >
        {query}
      </pre>

      <Button
        type="submit"
        kind="secondary"
        grouped
        className={styles.executeButton}
      >
        Execute
      </Button>
    </form>
  );
}

export default QueryInput;
