import { useAppDispatch } from '@webapp/redux/hooks';
import React, { useEffect, useLayoutEffect, useRef } from 'react';
import { loadCurrentConfig } from '@webapp/redux/reducers/currentConfig';
import { connect } from 'react-redux';
import Button from '@webapp/ui/Button';
import { addNotification } from '@webapp/redux/reducers/notifications';
import { RootState } from '@webapp/redux/store';
import styles from './CurrentConfig.module.scss';
import { Prism } from '../util/prism';

type PropType = {
  config: string;
};

function CurrentConfig(props: PropType) {
  const { config } = props;
  const codeRef = useRef(null);
  const dispatch = useAppDispatch();

  useEffect(() => {
    dispatch(loadCurrentConfig());
  }, []);

  useEffect(() => {
    if (config && codeRef.current) {
      Prism.highlightElement(codeRef.current);
    }
  }, [config]);

  function copyConfigToClipboard() {
    navigator.clipboard
      .writeText(config)
      .then(() =>
        dispatch(
          addNotification({
            type: 'success',
            title: 'Success',
            message: 'The configuration was copied to clipboard',
          })
        )
      )
      .catch(() =>
        dispatch(
          addNotification({
            type: 'danger',
            title: 'Failed',
            message: 'Failed to copy configuration to clipboard',
          })
        )
      );
  }
  return (
    <div className={styles.currentConfigApp}>
      <h2 className={styles.header}>Configuration</h2>
      <div>
        <Button kind="secondary" onClick={() => copyConfigToClipboard()}>
          Copy to clipboard
        </Button>
      </div>
      <pre className={styles.config}>
        <code ref={codeRef} className="language-yaml">
          {config}
        </code>
      </pre>
    </div>
  );
}

const selectYamlConfig = (config: RootState['currentConfig']['data']) =>
  config.yaml;

const mapStateToProps = (state: RootState) => ({
  config: selectYamlConfig(state.currentConfig.data),
});

export default connect(mapStateToProps)(CurrentConfig);
