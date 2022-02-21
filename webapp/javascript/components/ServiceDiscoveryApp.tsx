import React, { useEffect, useState } from 'react';
import { connect } from 'react-redux';
import { bindActionCreators } from 'redux';
import { fetchServiceDiscoveryAppData } from '../redux/actions';

type Target = {
  DiscoveredLabels: Record<string, string>;
  Labels: Record<string, string>;
  Job: string;
  Url: string;
  GlobalUrl: string;
  LastError: string;
  LastScrape: string;
  LastScrapeDuration: number;
  Health: string;
};

const ServiceDiscoveryApp = (props) => {
  const { actions, data, url } = props;
  const [groups, setGroups] = useState({});

  useEffect(() => {
    actions.fetchServiceDiscoveryAppData(url);
  }, []);

  useEffect(() => {
    if (data) {
      const acc = data.reduce((acc, next) => {
        if (!acc[next.Job]) {
          acc[next.Job] = [];
        }
        acc[next.Job].push(next);
        return acc;
      }, {});
      setGroups(acc);
    }
  }, [data]);

  function getUpCount(targets: Target[]) {
    return targets.filter((t) => t.Health === 'up').length;
  }
  return (
    <div
      className="p-2"
      style={{
        fontSize: '12px',
        width: '100%',
        height: '100%',
        overflow: 'auto',
        margin: '0 0 0 20px',
      }}
    >
      {Object.keys(groups).map((group) => (
        <CollapsibleSection
          title={`${groups[group][0].Job} (${getUpCount(groups[group])}/${
            groups[group].length
          }) up`}
          collapsed={false}
        >
          <div
            style={{
              display: 'grid',
              gridTemplateColumns: 'repeat( auto-fill, 400px)',
              gridGap: '10px',
            }}
          >
            {groups[group].map((target) => (
              /* eslint-disable-next-line react/jsx-props-no-spreading */
              <Target {...target} />
            ))}
          </div>
        </CollapsibleSection>
      ))}
    </div>
  );
};

const mapStateToProps = (state) => ({
  data: state.root.serviceDiscovery.data,
  url: 'targets',
});

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators(
    {
      fetchServiceDiscoveryAppData,
    },
    dispatch
  ),
});

export default connect(
  mapStateToProps,
  mapDispatchToProps
)(ServiceDiscoveryApp);

const CollapsibleSection = ({ children, title, collapsed }) => {
  const [isCollapsed, setCollapsed] = useState(collapsed);
  return (
    <div>
      <h2>
        <button
          onClick={() => setCollapsed(!isCollapsed)}
          style={{ all: 'unset' }}
        >
          {title}
        </button>
      </h2>
      <div
        style={{
          transition: 'all 500ms',
          maxHeight: isCollapsed ? '0px' : '2000px',
          maxWidth: '100%',
          overflow: 'auto',
        }}
      >
        {children}
      </div>
    </div>
  );
};
const Target = ({
  DiscoveredLabels,
  Labels,
  Job,
  Url,
  GlobalUrl,
  LastError,
  LastScrape,
  LastScrapeDuration,
  Health,
}: Target) => {
  return (
    <div
      style={{
        display: 'grid',
        gridTemplateColumns: '80px 1fr',
        margin: '20px 0',
        background: 'black',
        color: 'white',
        padding: '10px',
        width: '400px',
        borderRadius: '10px',
        columnGap: '10px',
        rowGap: '5px',
        height: 'fit-content',
        boxShadow: '5px 5px 15px 5px rgba(255, 255, 255 ,0.1)',
      }}
    >
      <div className="t-header">Discovered labels</div>
      <div className="t-value" style={{ maxHeight: '200px', overflow: 'auto' }}>
        {Object.keys(DiscoveredLabels).map((key) => (
          <Badge background="blueviolet">{`${key}=${DiscoveredLabels[key]}`}</Badge>
        ))}
      </div>
      <div className="t-header">Labels</div>
      <div className="t-value">
        {Object.keys(Labels).map((key) => (
          <Badge background="blueviolet">{`${key}=${Labels[key]}`}</Badge>
        ))}
      </div>
      <div className="t-header">Job</div>
      <div className="t-value">{Job}</div>
      <div className="t-header">Scrape Url</div>
      <div className="t-value">{Url}</div>
      <div className="t-header">Global Url</div>
      <div className="t-value">{GlobalUrl}</div>
      <div className="t-header">Last scrape</div>
      <div className="t-value">{LastScrape}</div>
      <div className="t-header">Last scrape duration</div>
      <div className="t-value">{LastScrapeDuration}</div>
      <div className="t-header">Last error</div>
      <div className="t-value">{LastError}</div>
      <div className="t-header">Health</div>
      <div className="t-value">
        <Badge background="green">{Health}</Badge>
      </div>
    </div>
  );
};

function Badge({
  children,
  background,
}: {
  children: string;
  background: string;
}) {
  return (
    <span
      className="t-badge"
      style={{
        background,
        fontWeight: 'bold',
        color: 'white',
        padding: '3px 5px',
        borderRadius: '5px',
        fontSize: 'small',
        margin: '0 2px 5px 2px',
        display: 'inline-block',
      }}
    >
      {children}
    </span>
  );
}
