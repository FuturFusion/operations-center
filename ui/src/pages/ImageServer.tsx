import Breadcrumbs from "components/Breadcrumbs";
import IncusImageList from "pages/IncusImageList";

const ImageServer = () => {
  return (
    <div className="d-flex flex-column">
      <Breadcrumbs />
      <div className="scroll-container flex-grow-1 p-3">
        <IncusImageList />
      </div>
    </div>
  );
};

export default ImageServer;
