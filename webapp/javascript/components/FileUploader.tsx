/* eslint-disable react/jsx-props-no-spreading */
import React, { useCallback, useState } from 'react';
import { useDropzone } from 'react-dropzone';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faTrash } from '@fortawesome/free-solid-svg-icons/faTrash';
import Button from '@ui/Button';
import styles from './FileUploader.module.scss';

interface Props {
  onUpload: (s: string) => void;
}
export default function FileUploader({ onUpload }: Props) {
  const [file, setFile] = useState();

  const onDrop = useCallback((acceptedFiles) => {
    if (acceptedFiles.length > 1) {
      throw new Error('Only a single file at a time is accepted.');
    }

    acceptedFiles.forEach((file) => {
      const reader = new FileReader();

      reader.onabort = () => console.log('file reading was aborted');
      reader.onerror = () => console.log('file reading has failed');
      reader.onload = () => {
        const binaryStr = reader.result;

        try {
          // ArrayBuffer -> JSON
          const s = JSON.parse(
            String.fromCharCode.apply(null, new Uint8Array(binaryStr))
          );
          setFile({ file, ...s });
          onUpload({ file, ...s });
        } catch (e) {
          console.log(e);
          alert(e);
        }
      };
      reader.readAsArrayBuffer(file);
    });
  }, []);
  const { getRootProps, getInputProps } = useDropzone({
    multiple: false,
    onDrop,
    accept: 'application/json',
  });

  const onRemove = () => {
    setFile(null);
    onUpload(null);
  };

  return (
    <section className={styles.container}>
      <div {...getRootProps()}>
        <input {...getInputProps()} />
        {file ? (
          <p>
            To analyze another file, drag and drop pyroscope JSON files here or
            click to select a file
          </p>
        ) : (
          <p>
            Drag and drop pyroscope JSON files here, or click to select a file
          </p>
        )}
      </div>
      {file && (
        <aside>
          Currently analyzing file {file.file.path}
          &nbsp;
          <Button icon={faTrash} onClick={onRemove}>
            Remove
          </Button>
        </aside>
      )}
    </section>
  );
}
