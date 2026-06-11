import { createHandler, StartServer } from "@solidjs/start/server";
import type { DocumentComponentProps } from "@solidjs/start/server";

function Document(props: DocumentComponentProps) {
  return (
    <html lang="ko">
      <head>{props.assets}</head>
      <body>
        <div id="app">{props.children}</div>
        {props.scripts}
      </body>
    </html>
  );
}

export default createHandler(() => <StartServer document={Document} />);
