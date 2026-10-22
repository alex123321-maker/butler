export interface GardenEvent { title:string; time:string; status:string; done:boolean }
export interface GardenContext {date:Date;events:GardenEvent[]}
export class ThoughtGarden {
constructor(host:HTMLElement,owner:{paused:boolean;motion:MediaQueryList},stage:HTMLElement);
setActive(active:boolean):void;
update(state:{base:string},context:GardenContext):void;
queueLayout():void;
highlight(index:number):void;
finish():void;
destroy():void;
}
