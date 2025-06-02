import { useAuth } from "context/authContext";
const Home = () => {
  const { isAuthenticated } = useAuth();

  return (
    <>
      <h1>Welcome to Operations Center</h1>
      {!isAuthenticated && (
        <div>
          Please log in using the navigation links on the left to continue.
        </div>
      )}
    </>
  );
};

export default Home;
