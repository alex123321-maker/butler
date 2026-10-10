package config

import "testing"

func TestSkillControlOnlyUsesFixedOperatorRoute(t *testing.T){
	env:=map[string]string{"BUTLER_POSTGRES_URL":"postgres://localhost:5432/test","BUTLER_REDIS_URL":"redis://localhost:6379/0","BUTLER_SKILL_CONTROL_URL":"http://tool-broker:8080/internal/skills"}
	settings:=[]Setting{{Key:"BUTLER_SKILL_CONTROL_URL",Value:"http://untrusted.example/collect"}}
	cfg,_,err:=loadOrchestratorLayered(envMap(env),settings);if err!=nil||cfg.SkillControlURL!=env["BUTLER_SKILL_CONTROL_URL"]{t.Fatal("operator skill route lost")};delete(env,"BUTLER_SKILL_CONTROL_URL");cfg,_,err=loadOrchestratorLayered(envMap(env),settings);if err!=nil||cfg.SkillControlURL!=""{t.Fatal("DB selected service-auth destination")}
	for _,endpoint:=range []string{"://invalid","http://user:pass@broker/internal/skills","http://broker/collect","http://broker/internal/skills?leak=yes","http://broker/internal/skills#ignore","file:///internal/skills"}{env["BUTLER_SKILL_CONTROL_URL"]=endpoint;if _,_,err=loadOrchestratorLayered(envMap(env),nil);err==nil{t.Fatalf("unsafe control endpoint accepted: %s",endpoint)}}
}
