import React, { useEffect, useRef, useState } from 'react';
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
  onCompare: (params: {
    from: string;
    until: string;
    leftFrom: string;
    leftTo: string;
    rightFrom: string;
    rightTo: string;
  }) => void;
  selection: {
    left: Selection;
    right: Selection;
  };
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
      label: '30 days prior',
      ms: 2592000 * 1000,
    },
  ],
];

const defaultPeriod = {
  label: 'Compare',
  ms: 0,
};

function usePrevious(selection: SideTimelineComparatorProps['selection']) {
  const ref = useRef<SideTimelineComparatorProps['selection']>();
  useEffect(() => {
    ref.current = selection;
  }, [selection]);
  return ref.current;
}

export default function SideTimelineComparator({
  onCompare,
  selection,
}: SideTimelineComparatorProps) {
  const refContainer = useRef(null);
  const [menuVisible, setMenuVisible] = useState(false);
  const [period, setPeriod] = useState(defaultPeriod);

  const prevSelection = usePrevious(selection);

  useEffect(() => {
    // reset previous period if selection has changed
    if (
      !menuVisible &&
      prevSelection &&
      (selection.left.from !== prevSelection.left.from ||
        selection.left.to !== prevSelection.left.to ||
        selection.right.from !== prevSelection.right.from ||
        selection.right.to !== prevSelection.right.to)
    ) {
      setPeriod(defaultPeriod);
    }
  }, [selection, prevSelection, menuVisible]);

  const [
    {
      xaxis: { from: comparisonSelectionFrom, to: comparisonSelectionTo },
    },
  ] = markingsFromSelection('single', {
    ...selection.right,
  } as Selection);

  const diff = comparisonSelectionTo - comparisonSelectionFrom;

  const fullLength = period.ms
    ? comparisonSelectionTo - (comparisonSelectionTo - period.ms - diff * 2)
    : null;

  const percent = fullLength ? (diff / fullLength) * 100 : null;

  const handleClick = ({ ms, label }: { ms: number; label: string }) => {
    setPeriod({ ms, label });
    onCompare({
      from: String(comparisonSelectionTo - ms - diff * 2),
      until: String(comparisonSelectionTo),
      leftFrom: String(comparisonSelectionTo - ms - diff),
      leftTo: String(comparisonSelectionTo - ms),
      rightFrom: String(comparisonSelectionFrom),
      rightTo: String(comparisonSelectionTo),
    });
  };

  const preview = percent ? (
    <div className={styles.preview}>
      <div className={styles.timeline}>
        <div className={styles.timelineBox}>
          <div
            className={styles.selection}
            style={{
              width: `${percent}%`,
              backgroundColor: selection.left.overlayColor.toString(),
              left: `${percent}%`,
            }}
          />
          <div
            style={{
              width: `${percent}%`,
              backgroundColor: selection.right.overlayColor.toString(),
              right: 0,
            }}
            className={styles.selection}
          />
        </div>
      </div>
      <div
        style={{ left: `${percent}%`, right: `${percent}%` }}
        className={styles.legend}
      >
        <div className={styles.legendLine} />
        <div className={styles.legendCaption}>{period.label}</div>
      </div>
    </div>
  ) : (
    <div>Please set the period</div>
  );

  return (
    <div className={styles.wrapper} ref={refContainer}>
      <Button
        data-testid="open-comparator-button"
        onClick={() => setMenuVisible(!menuVisible)}
      >
        {period.label}
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
                <div className={styles.subtitle}>Set baseline to:</div>
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
                                period.label === b.label
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
                <div className={styles.subtitle}>Preview</div>
                {preview}
              </PopoverBody>
            </>
          ) : null}
        </Popover>
      </Portal>
    </div>
  );
}
