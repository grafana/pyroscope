import React, { useRef, useState } from 'react';
import classNames from 'classnames/bind';
import Button from '@webapp/ui/Button';
import { Popover, PopoverBody } from '@webapp/ui/Popover';
import { Portal } from '@webapp/ui/Portal';
import { faChevronDown } from '@fortawesome/free-solid-svg-icons/faChevronDown';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  markingsFromSelection,
  Selection,
} from '@webapp/components/TimelineChart/markings';
import styles from './styles.module.scss';

interface SideTimelineComparatorProps {
  comparisonSelection: {
    from: string;
    to: string;
  };
  onCompare: (params: {
    from: string;
    until: string;
    leftFrom: string;
    leftTo: string;
    rightFrom: string;
    rightTo: string;
  }) => void;
}

const cx = classNames.bind(styles);

const buttons = [
  [
    {
      label: '1 hour prior',
      ms: 3600 * 1000,
    },
    {
      label: '12 hour prior',
      ms: 43200 * 1000,
    },
    {
      label: '24 hour prior',
      ms: 86400 * 1000,
    },
  ],
  [
    {
      label: '1 week prior',
      ms: 604800 * 1000,
    },
    {
      label: '2 weeks prior',
      ms: 1209600 * 1000,
    },
    {
      label: '1 month prior',
      // TODO: 30days, needs clarification
      ms: 2592000 * 1000,
    },
  ],
];

export default function SideTimelineComparator({
  comparisonSelection,
  onCompare,
}: SideTimelineComparatorProps) {
  const refContainer = useRef(null);
  const [menuVisible, setMenuVisible] = useState(false);
  const [buttonCaption, setButtonCaption] = useState('Compare');

  const [
    {
      xaxis: { from: comparisonSelectionFrom, to: comparisonSelectionTo },
    },
  ] = markingsFromSelection('single', {
    ...comparisonSelection,
  } as Selection);

  const diff = comparisonSelectionTo - comparisonSelectionFrom;

  const handleClick = ({ ms, label }: { ms: number; label: string }) => {
    setButtonCaption(label);
    onCompare({
      from: String(comparisonSelectionTo - ms * 2),
      until: String(comparisonSelectionTo),
      rightFrom: String(comparisonSelectionFrom),
      rightTo: String(comparisonSelectionTo),
      leftFrom: String(comparisonSelectionTo - ms - diff),
      leftTo: String(comparisonSelectionTo - ms),
    });
    setMenuVisible(false);
  };

  return (
    <div className={styles.wrapper} ref={refContainer}>
      <Button
        data-testid="open-comparator-button"
        onClick={() => setMenuVisible(!menuVisible)}
      >
        {buttonCaption}
        <FontAwesomeIcon
          className={styles.openButtonIcon}
          icon={faChevronDown}
        />
      </Button>
      <Portal container={refContainer.current}>
        <Popover
          anchorPoint={{ x: 'calc(100% - 300px)', y: 42 }}
          isModalOpen
          setModalOpenStatus={() => setMenuVisible(false)}
          className={cx({ [styles.menu]: true, [styles.hidden]: !menuVisible })}
        >
          {menuVisible ? (
            <>
              <PopoverBody className={styles.body}>
                <div>Set baseline to:</div>
                <div className={styles.buttons}>
                  {buttons.map((arr, i) => {
                    return (
                      <div
                        key={`preset-${i + 1}`}
                        className={styles.buttonsCol}
                      >
                        {arr.map((b) => {
                          return (
                            <Button
                              kind={
                                buttonCaption === b.label
                                  ? 'secondary'
                                  : 'default'
                              }
                              disabled={diff > b.ms}
                              key={b.label}
                              data-testid={b.label}
                              onClick={() => {
                                handleClick(b);
                              }}
                              className={styles.priorButton}
                            >
                              {b.label}
                            </Button>
                          );
                        })}
                      </div>
                    );
                  })}
                </div>
              </PopoverBody>
            </>
          ) : null}
        </Popover>
      </Portal>
    </div>
  );
}
