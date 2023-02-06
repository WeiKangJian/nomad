import Controller from '@ember/controller';
import { action } from '@ember/object';

export default class GenerationController extends Controller {
  @action
  async handleClick() {
    window.localStorage.gVal = JSON.stringify(this.get('model'));
  }
}
