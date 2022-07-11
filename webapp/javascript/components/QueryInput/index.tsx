import React, {
  useState,
  useEffect,
  useRef,
  Dispatch,
  SetStateAction,
} from 'react';
import classNames from 'classnames/bind';
import Button from '@webapp/ui/Button';
import ReactDOM from 'react-dom';
import { useWindowWidth } from '@react-hook/window-size';
import TextareaAutosize from 'react-textarea-autosize';
import { Prism } from '@webapp/util/prism';
import { Query, brandQuery } from '@webapp/models/query';
import OutsideClickHandler from 'react-outside-click-handler';
// eslint-disable-next-line css-modules/no-unused-class
import styles from './QueryInput.module.scss';

const cx = classNames.bind(styles);

const inlineSvg = `<svg viewPort="0 0 12 12" version="1.1" xmlns="http://www.w3.org/2000/svg"> <line x1="1" y1="11" x2="11" y2="1" stroke="white" stroke-width="2"/> <line x1="1" y1="1" x2="11" y2="11" stroke="white" stroke-width="2"/> </svg>`;

interface QueryInputProps {
  initialQuery: Query;
  onSubmit: (query: string) => void;
}

const ItemMenu = () => {
  const [visible, toggle] = useState(false);
  return (
    <OutsideClickHandler onOutsideClick={() => toggle(false)}>
      <div
        role="none"
        onClick={() => toggle(!visible)}
        className={styles.menuInnerWrapper}
      >
        <div
          className={cx({ menu: true, visible })}
          aria-hidden="true"
          onClick={(e) => {
            e.stopPropagation();
          }}
        >
          <div className={styles.menuItem}>Menu Item I</div>
          <div className={styles.menuItem}>Menu Item II</div>
          <div className={styles.menuItem}>Menu Item III</div>
        </div>
      </div>
    </OutsideClickHandler>
  );
};

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

function decorateAttrValues(setQuery: Dispatch<SetStateAction<Query>>) {
  Prism.hooks.add('after-highlight', (env) => {
    const list = env.element.querySelectorAll(
      'span.token.label-value.attr-value'
    );

    const handleClick = (e: MouseEvent) => {
      const eTarget = e.target as ShamefulAny;
      const attrValue = eTarget?.parentElement?.innerText;
      const attrPunctuation =
        eTarget?.parentElement?.previousElementSibling?.innerText;
      const attrName =
        eTarget?.parentElement?.previousElementSibling?.previousElementSibling
          ?.innerText;

      const regExp = new RegExp(
        `\\b${attrName}\\b[\\s|=|~|!]{1,4}["'\`](.*?(${attrValue.replace(
          /['"\s]+/g,
          ''
        )}).*?)["'\`]`,
        'gmi'
      );

      setQuery((query: ShamefulAny) => {
        const red = query.match(regExp);

        if (red?.[0]) {
          return query.replace(red?.[0], '');
        }

        return query;
      });
    };

    Array.from(list).forEach((item) => {
      const closeBox = item.querySelector(`span[data-role="close"]`);
      const menuBox = item.querySelector(`span[data-role="menu"]`);

      if (!closeBox) {
        const closeDiv = createElem(
          'span',
          'close',
          styles.deletePill,
          inlineSvg
        );

        closeDiv.onclick = (e) => handleClick(e);

        item.appendChild(closeDiv);
      }

      if (!menuBox) {
        const menuWrapper = createElem('span', 'menu', styles.itemMenuWrapper);

        ReactDOM.render(<ItemMenu />, menuWrapper);

        item.appendChild(menuWrapper);
      }
    });
  });
}

function QueryInput({ initialQuery, onSubmit }: QueryInputProps) {
  const windowWidth = useWindowWidth();
  const [query, setQuery] = useState(initialQuery);
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
    decorateAttrValues(setQuery);

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
