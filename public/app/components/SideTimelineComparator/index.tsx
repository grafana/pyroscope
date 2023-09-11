import React, { useRef, useState } from 'react';
import classNames from 'classnames/bind';
import Button from '@pyroscope/ui/Button';
import { Popover, PopoverBody } from '@pyroscope/ui/Popover';
import { Portal } from '@pyroscope/ui/Portal';
import { faChevronDown } from '@fortawesome/free-solid-svg-icons/faChevronDown';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { Selection } from '@pyroscope/components/TimelineChart/markings';
import { getSelectionBoundaries } from '@pyroscope/components/TimelineChart/SyncTimelines/getSelectionBoundaries';
import { comparisonPeriods } from './periods';
import styles from './styles.module.scss';

const cx = classNames.bind(styles);

type Boudaries = {
  from: string;
  until: string;
  leftFrom: string;
  leftTo: string;
  rightFrom: string;
  rightTo: string;
};

interface SideTimelineComparatorProps {
  onCompare: (params: Boudaries) => void;
  selection: {
    left: Selection;
    right: Selection;
    from: string;
    until: string;
  };
  comparisonMode: {
    active: boolean;
    period: {
      label: string;
      ms: number;
    };
  };
  setComparisonMode: (
    params: SideTimelineComparatorProps['comparisonMode']
  ) => void;
}

const getNewBoundaries = ({
  selection,
  period,
}: {
  selection: SideTimelineComparatorProps['selection'];
  period: SideTimelineComparatorProps['comparisonMode']['period'];
}) => {
  const { from: comparisonSelectionFrom, to: comparisonSelectionTo } =
    getSelectionBoundaries(selection.right);

  const diff = comparisonSelectionTo - comparisonSelectionFrom;

  return {
    from: String(comparisonSelectionTo - period.ms - diff * 2),
    until: String(comparisonSelectionTo),
    leftFrom: String(comparisonSelectionTo - period.ms - diff),
    leftTo: String(comparisonSelectionTo - period.ms),
    rightFrom: String(comparisonSelectionFrom),
    rightTo: String(comparisonSelectionTo),
  };
};

export default function SideTimelineComparator({
  onCompare,
  selection,
  setComparisonMode,
  comparisonMode,
}: SideTimelineComparatorProps) {
  const [previousSelection, setPreviousSelection] = useState<Boudaries | null>(
    null
  );
  const refContainer = useRef(null);
  const [menuVisible, setMenuVisible] = useState(false);

  const { active, period } = comparisonMode;

  const { from: comparisonSelectionFrom, to: comparisonSelectionTo } =
    getSelectionBoundaries(selection.right);

  const diff = comparisonSelectionTo - comparisonSelectionFrom;

  const fullLength =
    comparisonSelectionTo - (comparisonSelectionTo - period.ms - diff * 2);

  const percent = fullLength ? (diff / fullLength) * 100 : null;

  const handleSelectPeriod = (period: { label: string; ms: number }) => {
    setComparisonMode({
      ...comparisonMode,
      period,
    });

    if (comparisonMode.active) {
      const newBoundaries = getNewBoundaries({ period, selection });

      onCompare(newBoundaries);
    }
  };

  const hanleToggleComparison = (e: React.ChangeEvent<HTMLInputElement>) => {
    const active = e.target.checked;

    if (active) {
      setPreviousSelection({
        from: selection.from,
        until: selection.until,
        leftFrom: selection.left.from,
        leftTo: selection.left.to,
        rightFrom: selection.right.from,
        rightTo: selection.right.to,
      });

      const newBoundaries = getNewBoundaries({ period, selection });

      onCompare(newBoundaries);
    } else if (previousSelection) {
      onCompare(previousSelection);
    }

    setComparisonMode({
      ...comparisonMode,
      active,
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
      <input
        onChange={hanleToggleComparison}
        checked={active}
        type="checkbox"
        className={styles.toggleCompare}
      />
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
      <span className={styles.caption}>&nbsp;&nbsp;to comparison</span>
      <Portal container={refContainer.current}>
        <Popover
          anchorPoint={{ x: 'calc(100% - 350px)', y: 42 }}
          isModalOpen
          setModalOpenStatus={() => setMenuVisible(false)}
          className={cx({ [styles.menu]: true, [styles.hidden]: !menuVisible })}
        >
          {menuVisible ? (
            <>
              <PopoverBody className={styles.body}>
                <div className={styles.subtitle}>
                  Set baseline&nbsp;
                  <span className={styles.periodLabel}>{period.label}</span>
                  &nbsp;to comparison
                </div>
                <div className={styles.buttons}>
                  {comparisonPeriods.map((arr, i) => {
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
                                handleSelectPeriod(b);
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
