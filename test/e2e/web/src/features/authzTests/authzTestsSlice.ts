import { createAppSlice } from "../../app/createAppSlice";
import { postHello, postHelloToSomeone } from "./authzTestsApi";

interface HelloState {
	readonly success: string;
	readonly error: string;
}

const initialHelloState = (): HelloState => ({
	success: "",
	error: ""
});

export interface UserSliceState {
	readonly sayHello: HelloState;
	readonly sayHelloToSomeone: HelloState;
}

const initialState: UserSliceState = {
	sayHello: initialHelloState(),
	sayHelloToSomeone: initialHelloState()
};

export const authzTestsSlice = createAppSlice({
	name: "authzTests",
	initialState,
	reducers: create => ({
		sayHello: create.asyncThunk(
			async () => {
				const response = await postHello();
				return response.result;
			},
			{
				pending: () => {
					console.log("sayHello pending...");
				},
				fulfilled: (state, action) => {
					state.sayHello.success = action.payload;
					state.sayHello.error = "";
				},
				rejected: (state, action) => {
					state.sayHello.error = action.error.message as string;
					state.sayHello.success = "";
				}
			}
		),
		sayHelloToSomeone: create.asyncThunk(
			async (toWhom: string) => {
				const response = await postHelloToSomeone(toWhom);
				return response.result;
			},
			{
				pending: () => {
					console.log("sayHelloToSomeone pending...");
				},
				fulfilled: (state, action) => {
					state.sayHelloToSomeone.success = action.payload;
					state.sayHelloToSomeone.error = "";
				},
				rejected: (state, action) => {
					state.sayHelloToSomeone.error = action.error.message as string;
					state.sayHelloToSomeone.success = "";
				}
			}
		)
	}),
	selectors: {
		selectHelloResult: authzTests => authzTests.sayHello,
		selectHelloToSomeoneResult: authzTests => authzTests.sayHelloToSomeone
	}
});

export const { sayHello, sayHelloToSomeone } =
  authzTestsSlice.actions;

export const { selectHelloResult, selectHelloToSomeoneResult } =
	authzTestsSlice.selectors;
