/* eslint-disable react/jsx-props-no-spreading, jsx-a11y/role-supports-aria-props */
import React, { useCallback, useState } from 'react';
import { useDropzone } from 'react-dropzone';
import Button from '@webapp/ui/Button';
import type { DropzoneOptions } from 'react-dropzone';
import { faFileUpload } from '@fortawesome/free-solid-svg-icons/faFileUpload';
import classNames from 'classnames/bind';
import Select from '@webapp/ui/Select';
import UploadIcon from './UploadIcon';
import styles from './FileUploader.module.scss';

const cx = classNames.bind(styles);

const isJSONFile = (file: File) =>
  file.name.match(/\.json$/) ||
  file.type === 'application/json' ||
  file.type === 'text/json';

const spyNames = {
  gospy: 'Go',
  pyspy: 'Python',
  phpspy: 'PHP',
  'pyroscope-rs': 'Rust',
  rbspy: 'Ruby',
  javaspy: 'Java',
  dotnetspy: '.NET',
  nodespy: 'NodeJS',
  ebpfspy: 'eBPF',
  other: 'Other',
};

const availableUnits = {
  samples: 'Samples',
  objects: 'Objects',
  bytes: 'Bytes',
};

type UploadArgsType = {
  file: File;
  spyName?: string;
  units?: string;
};
interface Props {
  setFile: ({ file, spyName, units }: UploadArgsType) => void;
  className?: string;
}

export default function FileUploader({ setFile: onUpload, className }: Props) {
  const [file, setFile] = useState<File>();
  const [spyName, setSpyName] = useState<string>('gospy');
  const [units, setUnits] = useState('samples');
  type onDrop = Required<DropzoneOptions>['onDrop'];
  const onDrop = useCallback<onDrop>(
    (acceptedFiles) => {
      if (acceptedFiles.length > 1) {
        throw new Error('Only a single file at a time is accepted.');
      }

      acceptedFiles.forEach((f) => {
        setFile(f);
      });
    },
    [setFile]
  );

  const showLanguageAndUnits = (file && !isJSONFile(file)) || false;

  const { getRootProps, getInputProps } = useDropzone({
    multiple: false,
    onDrop,
  });

  const descriptionOrFilename = file
    ? file.name
    : 'Upload profile in pprof, json, or collapsed format';

  const onUploadClick = () => {
    if (file) {
      let upload;

      if (showLanguageAndUnits) {
        upload = { file, spyName, units };
      } else {
        upload = { file };
      }

      onUpload(upload);
    }
  };

  return (
    <>
      <section className={`${styles.container} ${className}`}>
        <div {...getRootProps()} className={styles.dragAndDropContainer}>
          <input {...getInputProps()} />
          <p className={styles.headerMain}>{descriptionOrFilename}</p>
          <div className={styles.iconContainer}>
            <UploadIcon />
          </div>
          <p className={styles.uploadBtnPreLabel}>
            Drag & Drop
            <span className={styles.uploadBtnPreLabel}>or</span>
          </p>
          <div className={styles.uploadBtnWrapper}>
            <Button
              kind="primary"
              className={cx({
                [styles.uploadButton]: true,
                [styles.disabled]: !!file,
              })}
              icon={faFileUpload}
              disabled={!!file}
            >
              Select a file
            </Button>
          </div>
        </div>
      </section>
      {showLanguageAndUnits && (
        <>
          <label htmlFor="spyName-input">
            Profile language:
            <Select
              ariaLabel="view"
              id="spyName-input"
              aria-placeholder="This would be used to apply color to the flamegraph"
              value={spyName}
              onChange={(e) => setSpyName(e.target.value)}
            >
              {Object.keys(spyNames).map((name, index) => (
                <option key={String(index) + name} value={name}>
                  {spyNames[name as keyof typeof spyNames]}
                </option>
              ))}
            </Select>
          </label>
          <label htmlFor="units-input">
            Profile units:
            <Select
              ariaLabel="view"
              id="units-input"
              aria-placeholder="This would be used to apply units to the flamegraph"
              value={units}
              onChange={(e) => setUnits(e.target.value)}
            >
              {Object.keys(availableUnits).map((unit) => (
                <option key={unit} value={unit}>
                  {availableUnits[unit as keyof typeof availableUnits]}
                </option>
              ))}
            </Select>
          </label>
        </>
      )}
      <div className={styles.uploadWrapper}>
        <Button kind="primary" disabled={!file} onClick={onUploadClick}>
          Save
        </Button>
      </div>
    </>
  );
}
