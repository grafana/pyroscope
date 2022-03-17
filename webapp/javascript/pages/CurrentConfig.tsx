import { useAppDispatch } from '@pyroscope/redux/hooks';
import React, { useEffect } from 'react';
import { loadCurrentConfig } from '@pyroscope/redux/reducers/currentConfig';
import { connect } from 'react-redux';
import Button from '@ui/Button';
import { addNotification } from '@pyroscope/redux/reducers/notifications';
import { RootState } from '@pyroscope/redux/store';
import styles from './CurrentConfig.module.scss';

type PropType = {
  config: string;
};

function CurrentConfig(props: PropType) {
  const { config } = props;
  const dispatch = useAppDispatch();

  useEffect(() => {
    dispatch(loadCurrentConfig());
  }, []);

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
      <pre className={styles.config}>{config}</pre>
    </div>
  );
}

const selectYamlConfig = (config: RootState['currentConfig']['data']) =>
  config.yaml;

const mapStateToProps = (state: RootState) => ({
  config: selectYamlConfig(state.currentConfig.data),
});

export default connect(mapStateToProps)(CurrentConfig);
