import React from 'react';
import { createTheme } from '@grafana/data';
import { FlameGraph } from '@grafana/flamegraph';
import { Button, Tooltip } from '@grafana/ui';

import useColorMode from '@pyroscope/hooks/colorMode.hook';
import { FlamegraphRenderer } from '@pyroscope/legacy/flamegraph/FlamegraphRenderer';
import ExportData from '@pyroscope/components/ExportData';
import type { Profile } from '@pyroscope/legacy/models';
import useExportToFlamegraphDotCom from '@pyroscope/components/exportToFlamegraphDotCom.hook';
import { SharedQuery } from '@pyroscope/legacy/flamegraph/FlameGraph/FlameGraphRenderer';

import {
  isExportToFlamegraphDotComEnabled,
  isGrafanaFlamegraphEnabled,
} from '@pyroscope/util/features';

import { flamebearerToDataFrameDTO } from '@pyroscope/util/flamebearer';

type Props = {
  profile?: Profile;
  dataTestId?: string;
  vertical?: boolean;
  sharedQuery?: SharedQuery;
  timelineEl?: React.ReactNode;
  diff?: boolean;
};

export function FlameGraphWrapper(props: Props) {
  const { colorMode } = useColorMode();
  const exportToFlamegraphDotComFn = useExportToFlamegraphDotCom(props.profile);

  if (isGrafanaFlamegraphEnabled) {
    const dataFrame = props.profile
      ? flamebearerToDataFrameDTO(
          props.profile.flamebearer.levels,
          props.profile.flamebearer.names,
          props.profile.metadata.units,
          Boolean(props.diff)
        )
      : undefined;

    let extraEl = <></>;

    // This is a bit weird but the typing is not great. It seems like flamegraph assumed profile can be undefined
    // but then ExportData won't work so not sure if the profile === undefined could actually happen.
    if (props.profile) {
      extraEl = (
        <ExportData
          flamebearer={props.profile}
          exportPNG
          exportJSON
          exportPprof
          exportHTML
          exportFlamegraphDotCom={isExportToFlamegraphDotComEnabled}
          exportFlamegraphDotComFn={exportToFlamegraphDotComFn}
          buttonEl={({ onClick }) => {
            return (
              <Tooltip content={'Export Data'}>
                <Button
                  // Ugly hack to go around globally defined line height messing up sizing of the button.
                  // Not sure why it happens even if everything is display: Block. To override it would
                  // need changes in Flamegraph which would be weird so this seems relatively sensible.
                  style={{ marginTop: -7 }}
                  icon={'download-alt'}
                  size={'sm'}
                  variant={'secondary'}
                  fill={'outline'}
                  onClick={onClick}
                />
              </Tooltip>
            );
          }}
        />
      );
    }

    return (
      <>
        {props.timelineEl}
        <FlameGraph
          getTheme={() => createTheme({ colors: { mode: colorMode } })}
          data={dataFrame}
          extraHeaderElements={extraEl}
          vertical={props.vertical}
          disableCollapsing={true}
        />
      </>
    );
  }

  let exportData = undefined;
  if (props.profile) {
    exportData = (
      <ExportData
        flamebearer={props.profile}
        exportPNG
        exportJSON
        exportPprof
        exportHTML
        exportFlamegraphDotCom={isExportToFlamegraphDotComEnabled}
        exportFlamegraphDotComFn={exportToFlamegraphDotComFn}
      />
    );
  }

  return (
    <FlamegraphRenderer
      showCredit={false}
      profile={props.profile}
      colorMode={colorMode}
      ExportData={exportData}
      data-testid={props.dataTestId}
      sharedQuery={props.sharedQuery}
    >
      {props.timelineEl}
    </FlamegraphRenderer>
  );
}
