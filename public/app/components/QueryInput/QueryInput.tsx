import React, { useState, useEffect, useRef } from 'react';
import { useWindowWidth } from '@react-hook/window-size';
import TextareaAutosize from 'react-textarea-autosize';
import cx from 'classnames';

import { Query, brandQuery } from '@pyroscope/models/query';
import { Prism } from '@pyroscope/util/prism';
import Button from '@pyroscope/ui/Button';

import styles from './QueryInput.module.scss';

interface QueryInputProps {
  initialQuery: Query;
  onSubmit: (query: string) => void;
}

/**
 * QueryInput refers to the input box used for queries
 */
export default function QueryInput({
  initialQuery,
  onSubmit,
}: QueryInputProps) {
  const windowWidth = useWindowWidth();
  const [query, setQuery] = useState(initialQuery);
  const codeRef = useRef<HTMLElement>(null);
  const textareaRef = useRef<ShamefulAny>(null);
  const [textAreaSize, setTextAreaSize] = useState({ width: 0, height: 0 });

  // If query updated upstream, most likely the application changed
  // So we prefer to use it
  useEffect(() => {
    setQuery(initialQuery);
  }, [initialQuery]);

  useEffect(() => {
    if (Prism && codeRef.current) {
      Prism.highlightElement(codeRef.current);
    }
  }, [query]);

  useEffect(() => {
    setTextAreaSize({
      width: textareaRef?.current?.offsetWidth || 0,
      height: textareaRef?.current?.offsetHeight || 0,
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
    if (e.key === 'Enter' || e.keyCode === 13 || e.keyCode === 10) {
      onFormSubmit(e);
    }
  };

  return (
    <form
      aria-label="query-input"
      className={styles.wrapper}
      onSubmit={onFormSubmit}
    >
      <pre
        className={cx(styles.highlight, 'language-promql')}
        aria-hidden="true"
      >
        <code
          className="language-promql"
          id="highlighting-content"
          ref={codeRef}
          style={textAreaSize}
        >
          {query}
        </code>
      </pre>
      <TextareaAutosize
        className={styles.input}
        ref={textareaRef}
        value={query}
        spellCheck="false"
        onChange={(e) => setQuery(brandQuery(e.target.value))}
        onKeyDown={handleTextareaKeyDown}
      />
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
