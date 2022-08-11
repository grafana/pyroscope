import React, { Children, useEffect, useState } from 'react';
import { Target } from '@webapp/models/targets';
import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import {
  loadTargets,
  selectTargetsData,
} from '@webapp/redux/reducers/serviceDiscovery';
import { formatDistance, parseISO } from 'date-fns';
import cx from 'classnames';
import Button from '@webapp/ui/Button';
import styles from './ServiceDiscovery.module.scss';

enum Status {
  healthy = 'healthy',
  info = 'info',
  error = 'error',
}

const ServiceDiscoveryApp = () => {
  const data = targetsToMap(useAppSelector(selectTargetsData));
  const dispatch = useAppDispatch();
  const [unavailableFilter, setUnavailableFilter] = useState(false);
  const [expandAll, setExpandAll] = useState(true);

  useEffect(() => {
    async function run() {
      await dispatch(loadTargets());
    }

    run();
  }, []);

  function getUpCount(targets: Target[]) {
    return targets.filter((t) => t.health === 'up').length;
  }

  return (
    <div className={styles.serviceDiscoveryApp}>
      <h2 className={styles.header}>Targets</h2>
      <div className={styles.buttonGroup}>
        <Button
          kind="secondary"
          grouped
          onClick={() => setUnavailableFilter(!unavailableFilter)}
        >
          {unavailableFilter ? 'Show All' : 'Show Unhealthy Only'}
        </Button>
        <Button
          kind="secondary"
          grouped
          onClick={() => setExpandAll(!expandAll)}
        >
          {expandAll ? 'Collapse All' : 'Expand All'}
        </Button>
      </div>

      <div>
        {Object.keys(data).length === 0 ? (
          <div>
            {'No pull-mode targets configured. See '}
            <a
              className={styles.link}
              href="https://pyroscope.io/docs/golang-pull-mode/"
              target="_blank"
              rel="noreferrer"
            >
              documentation
            </a>
            {' for information on how to add targets.'}
          </div>
        ) : (
          Object.keys(data).map((job) => {
            const children = data[job].map((target) => {
              const targetElem = (
                /* eslint-disable-next-line react/jsx-props-no-spreading */
                <TargetComponent {...target} key={target.url} />
              );
              if (unavailableFilter) {
                if (target.health !== 'up') {
                  return targetElem;
                }
                return null;
              }
              return targetElem;
            });

            return (
              <CollapsibleSection
                title={`${data[job][0].job} (${getUpCount(data[job])}/${
                  data[job].length
                }) up`}
                key={job}
                open={expandAll}
              >
                {children}
              </CollapsibleSection>
            );
          })
        )}
      </div>
    </div>
  );
};

const CollapsibleSection = ({ children, title, open }: ShamefulAny) => {
  return Children.count(children.filter((c: ShamefulAny) => c)) > 0 ? (
    <details open={open}>
      <summary className={styles.collapsibleHeader}>{title}</summary>
      <div className={styles.collapsibleSection}>
        <table className={styles.target}>
          <thead>
            <tr>
              <th className={styles.tableCell} style={{ width: '25%' }}>
                Scrape URL
              </th>
              <th className={styles.tableCell} style={{ width: '10%' }}>
                Health
              </th>
              <th className={styles.tableCell} style={{ width: '10%' }}>
                Discovered labels
              </th>
              <th className={styles.tableCell} style={{ width: '10%' }}>
                Labels
              </th>
              <th className={styles.tableCell} style={{ width: '10%' }}>
                Last scrape
              </th>
              <th className={styles.tableCell} style={{ width: '10%' }}>
                Scrape duration
              </th>
              <th className={styles.tableCell} style={{ width: '25%' }}>
                Last error
              </th>
            </tr>
          </thead>
          <tbody>{children}</tbody>
        </table>
      </div>
    </details>
  ) : null;
};

function formatDuration(input: string): string {
  const a = input.match(/[a-zA-Z]+$/);
  const b = a ? a[0] : '';
  return `${parseFloat(input).toFixed(2)} ${b}`;
}

const TargetComponent = ({
  discoveredLabels,
  labels,
  url,
  lastError,
  lastScrape,
  lastScrapeDuration,
  health,
}: Target) => {
  return (
    <tr>
      <td className={styles.tableCell}>{url}</td>
      <td className={styles.tableCell}>
        <Badge status={health === 'up' ? Status.healthy : Status.error}>
          {health}
        </Badge>
      </td>
      <td className={styles.tableCell}>
        {Object.keys(discoveredLabels).map((key) => (
          <Badge
            status={Status.info}
            key={key}
          >{`${key}=${discoveredLabels[key]}`}</Badge>
        ))}
      </td>
      <td className={styles.tableCell}>
        {Object.keys(labels).map((key) => (
          <Badge
            status={Status.info}
            key={key}
          >{`${key}=${labels[key]}`}</Badge>
        ))}
      </td>
      <td className={styles.tableCell} title={lastScrape}>
        {formatDistance(parseISO(lastScrape), new Date())} ago
      </td>
      <td className={styles.tableCell}>{formatDuration(lastScrapeDuration)}</td>
      <td className={styles.tableCell}>{lastError || '-'}</td>
    </tr>
  );
};

const Badge = ({ children, status }: { children: string; status: Status }) => {
  function getStatusClass(status: ShamefulAny) {
    switch (status) {
      case Status.healthy:
        return styles.healthy;
      case Status.info:
        return styles.info;
      case Status.error:
        return styles.error;
      default:
        return styles.info;
    }
  }
  return (
    <span className={cx(styles.badge, getStatusClass(status))}>{children}</span>
  );
};

type TargetRecord = Record<string, Target[]>;
const targetsToMap: (state: Target[]) => TargetRecord = (state) => {
  const acc = state.reduce((acc: TargetRecord, next: Target) => {
    if (!acc[next.job]) {
      acc[next.job] = [];
    }
    acc[next.job].push(next);
    return acc;
  }, {} as TargetRecord);
  return acc;
};

export default ServiceDiscoveryApp;
