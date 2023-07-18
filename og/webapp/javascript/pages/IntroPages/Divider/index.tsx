import React from 'react';
import styles from './Divider.module.css';

function Divider() {
  return (
    <div className={styles.divider}>
      <div>
        <hr />
      </div>
      <div>or</div>
      <div>
        <hr />
      </div>
    </div>
  );
}

export default Divider;
