import React, { useRef, useState } from 'react';
import classNames from 'classnames/bind';
import Button from '@webapp/ui/Button';
import { Popover, PopoverBody } from '@webapp/ui/Popover';
import { Portal } from '@webapp/ui/Portal';
import styles from './styles.module.scss';
import { faChevronDown } from '@fortawesome/free-solid-svg-icons/faChevronDown';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

const cx = classNames.bind(styles);

const buttons = [
  [
    {
      label: '1 hour prior',
      value: '1h',
    },
    {
      label: '12 hour prior',
      value: '12h',
    },
    {
      label: '24 hour prior',
      value: '24h',
    },
  ],
  [
    {
      label: '1 week prior',
      value: '7d',
    },
    {
      label: '2 weeks prior',
      value: '14d',
    },
    {
      label: '1 month prior',
      value: '1M',
    },
  ],
];

export default function SideTimelineComparator() {
  const refContainer = useRef(null);
  const [menuVisible, setMenuVisible] = useState(false);

  return (
    <div className={styles.wrapper} ref={refContainer}>
      <Button
        data-testid="open-comparator-button"
        onClick={() => setMenuVisible(!menuVisible)}
      >
        {'1 day prior'}
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
          className={cx({ menu: true, [styles.hidden]: !menuVisible })}
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
                              key={b.value}
                              data-testid={b.label}
                              onClick={() => {}}
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
